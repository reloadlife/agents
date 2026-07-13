package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/reloadlife/agents/internal/agent"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
	"github.com/reloadlife/agents/internal/redact"
)

// Manager owns the queue and running jobs.
type Manager struct {
	cfg   *config.Config
	store *Store
	log   *slog.Logger

	mu       sync.Mutex
	running  int
	cancels  map[string]context.CancelFunc
	subs     map[string]map[chan string]struct{} // job id -> log subscribers
	wake     chan struct{}
	queue    []string // job ids
	stop     chan struct{}
	wg       sync.WaitGroup
}

func NewManager(cfg *config.Config, store *Store, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		cfg:     cfg,
		store:   store,
		log:     log,
		cancels: map[string]context.CancelFunc{},
		subs:    map[string]map[chan string]struct{}{},
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
}

func (m *Manager) Start() {
	_ = m.store.MarkInterrupted()
	// re-queue any leftover queued / awaiting not needed on boot
	m.wg.Add(1)
	go m.loop()
}

func (m *Manager) Stop() {
	close(m.stop)
	m.mu.Lock()
	for id, cancel := range m.cancels {
		cancel()
		_ = id
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stop:
			return
		case <-m.wake:
		case <-time.After(2 * time.Second):
		}
		m.tryStartNext()
	}
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) Create(req CreateRequest) (*Job, error) {
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt required")
	}
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if req.Cwd == "" {
		req.Cwd = "."
	}
	acfg, ok := m.cfg.Agent(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", req.Agent)
	}
	if acfg.Bin == "" {
		return nil, fmt.Errorf("agent %q not configured", req.Agent)
	}
	cwdAbs, err := pathallow.Resolve(m.cfg.WorkspaceRoot, req.Cwd, m.cfg.Allow.Paths)
	if err != nil {
		return nil, err
	}
	// ensure cwd exists
	if st, err := os.Stat(cwdAbs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("cwd does not exist or is not a directory: %s", req.Cwd)
	}

	timeout := m.cfg.DefaultTimeoutDur
	if req.Timeout != "" {
		d, err := time.ParseDuration(req.Timeout)
		if err != nil {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		if d > m.cfg.MaxTimeoutDur {
			d = m.cfg.MaxTimeoutDur
		}
		timeout = d
	}

	caps := req.Caps
	if len(caps) == 0 {
		caps = append([]string{}, m.cfg.Caps.Default...)
	}
	needConfirm := false
	for _, c := range caps {
		if m.cfg.IsElevatedCap(c) {
			needConfirm = true
			break
		}
	}

	id := "j_" + ulid.Make().String()
	now := time.Now().UTC()
	j := &Job{
		ID:        id,
		Title:     req.Title,
		Prompt:    req.Prompt,
		Agent:     req.Agent,
		Cwd:       req.Cwd,
		CwdAbs:    cwdAbs,
		Caps:      caps,
		Timeout:   timeout.String(),
		CreatedAt: now,
	}
	if needConfirm {
		j.State = StateAwaitingConfirm
		j.Confirm = randomToken(16)
	} else {
		j.State = StateQueued
	}
	if err := m.store.Put(j); err != nil {
		return nil, err
	}
	if j.State == StateQueued {
		m.mu.Lock()
		m.queue = append(m.queue, j.ID)
		m.mu.Unlock()
		m.signal()
	}
	return j, nil
}

func (m *Manager) Confirm(id, token string) (*Job, error) {
	j, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if j.State != StateAwaitingConfirm {
		return nil, fmt.Errorf("job is not awaiting confirm (state=%s)", j.State)
	}
	if token == "" || token != j.Confirm {
		return nil, fmt.Errorf("invalid confirm token")
	}
	j.State = StateQueued
	j.Confirm = ""
	if err := m.store.Put(j); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.queue = append(m.queue, j.ID)
	m.mu.Unlock()
	m.signal()
	return j, nil
}

func (m *Manager) Get(id string) (*Job, error) { return m.store.Get(id) }
func (m *Manager) List(limit int) ([]*Job, error) { return m.store.List(limit) }
func (m *Manager) ReadLog(id string) ([]byte, error) { return m.store.ReadLog(id) }

func (m *Manager) Cancel(id string) (*Job, error) {
	j, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if j.State.Terminal() {
		return j, nil
	}
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	// remove from queue
	nq := m.queue[:0]
	for _, qid := range m.queue {
		if qid != id {
			nq = append(nq, qid)
		}
	}
	m.queue = nq
	m.mu.Unlock()

	if j.State == StateQueued || j.State == StateAwaitingConfirm {
		now := time.Now().UTC()
		j.State = StateCancelled
		j.EndedAt = &now
		j.Error = "cancelled"
		_ = m.store.Put(j)
	}
	return m.store.Get(id)
}

// SubscribeLog returns a channel of log lines (already redacted) and an unsubscribe func.
func (m *Manager) SubscribeLog(id string) (<-chan string, func()) {
	ch := make(chan string, 64)
	m.mu.Lock()
	if m.subs[id] == nil {
		m.subs[id] = map[chan string]struct{}{}
	}
	m.subs[id][ch] = struct{}{}
	m.mu.Unlock()
	unsub := func() {
		m.mu.Lock()
		if set, ok := m.subs[id]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(m.subs, id)
			}
		}
		m.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}

func (m *Manager) publish(id, line string) {
	line = redact.Line(line)
	_ = m.store.AppendLog(id, line)
	m.mu.Lock()
	defer m.mu.Unlock()
	for ch := range m.subs[id] {
		select {
		case ch <- line:
		default:
			// drop if slow consumer
		}
	}
}

func (m *Manager) tryStartNext() {
	m.mu.Lock()
	if m.running >= m.cfg.MaxConcurrentJobs || len(m.queue) == 0 {
		m.mu.Unlock()
		return
	}
	id := m.queue[0]
	m.queue = m.queue[1:]
	m.running++
	m.mu.Unlock()

	j, err := m.store.Get(id)
	if err != nil || j.State != StateQueued {
		m.mu.Lock()
		m.running--
		m.mu.Unlock()
		m.signal()
		return
	}
	go m.execute(j)
}

func (m *Manager) execute(j *Job) {
	defer func() {
		m.mu.Lock()
		m.running--
		delete(m.cancels, j.ID)
		m.mu.Unlock()
		m.signal()
	}()

	timeout := m.cfg.DefaultTimeoutDur
	if j.Timeout != "" {
		if d, err := time.ParseDuration(j.Timeout); err == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	m.mu.Lock()
	m.cancels[j.ID] = cancel
	m.mu.Unlock()

	now := time.Now().UTC()
	j.State = StateRunning
	j.StartedAt = &now
	_ = m.store.Put(j)
	m.publish(j.ID, fmt.Sprintf("==> starting agent=%s cwd=%s", j.Agent, j.Cwd))

	acfg, ok := m.cfg.Agent(j.Agent)
	if !ok {
		m.finish(j, 1, "", fmt.Errorf("unknown agent"))
		return
	}

	w := &lineWriter{fn: func(line string) { m.publish(j.ID, line) }}
	code, err := agent.Run(ctx, acfg, j.Agent, j.CwdAbs, j.Prompt, w)
	_ = w.Flush()

	if ctx.Err() == context.Canceled {
		m.finish(j, 130, "cancelled", nil)
		// ensure state cancelled
		jj, _ := m.store.Get(j.ID)
		if jj != nil && jj.State != StateCancelled {
			end := time.Now().UTC()
			jj.State = StateCancelled
			jj.EndedAt = &end
			code := 130
			jj.ExitCode = &code
			_ = m.store.Put(jj)
		}
		return
	}
	if ctx.Err() == context.DeadlineExceeded {
		m.finish(j, 124, "timeout", nil)
		return
	}
	summary := ""
	if err != nil && code == 1 {
		m.finish(j, code, err.Error(), err)
		return
	}
	m.finish(j, code, summary, nil)
}

func (m *Manager) finish(j *Job, code int, summary string, runErr error) {
	end := time.Now().UTC()
	j.EndedAt = &end
	j.ExitCode = &code
	j.Summary = summary
	if runErr != nil {
		j.Error = runErr.Error()
	}
	if code == 0 {
		j.State = StateSucceeded
	} else if code == 130 {
		j.State = StateCancelled
	} else {
		j.State = StateFailed
		if j.Error == "" {
			j.Error = fmt.Sprintf("exit code %d", code)
		}
	}
	_ = m.store.Put(j)
	_ = m.store.WriteResult(j.ID, Result{ExitCode: code, Summary: summary, Error: j.Error})
	m.publish(j.ID, fmt.Sprintf("==> finished state=%s exit=%d", j.State, code))
	m.log.Info("job finished", "id", j.ID, "state", j.State, "exit", code)
}

// RunningCount for status.
func (m *Manager) Stats() (running, queued int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running, len(m.queue)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// lineWriter splits on newlines and emits complete lines.
type lineWriter struct {
	buf []byte
	fn  func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := indexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.fn(line)
	}
	return len(p), nil
}

func (w *lineWriter) Flush() error {
	if len(w.buf) > 0 {
		w.fn(string(w.buf))
		w.buf = nil
	}
	return nil
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// Ensure lineWriter implements io.Writer
var _ io.Writer = (*lineWriter)(nil)
