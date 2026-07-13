package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// Mode of how the agent binary is launched inside tmux.
type Mode string

const (
	// ModeTTY = interactive terminal (subscription Claude UI). Default.
	ModeTTY Mode = "tty"
	// ModePrint = non-interactive -p style (API credits) — only if explicitly requested.
	ModePrint Mode = "print"
)

type State string

const (
	StateRunning State = "running"
	StateExited  State = "exited"
	StateKilled  State = "killed"
)

type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Agent     string    `json:"agent"`
	Mode      Mode      `json:"mode"`
	Cwd       string    `json:"cwd"`
	CwdAbs    string    `json:"cwd_abs"`
	Tmux      string    `json:"tmux"` // tmux session name
	State     State     `json:"state"`
	Prompt    string    `json:"prompt,omitempty"` // optional seed text for TTY
	CreatedAt time.Time `json:"created_at"`
	// How you attach (filled on create/get)
	Attach     string `json:"attach"`
	SSHAttach  string `json:"ssh_attach,omitempty"`
	PTYPath    string `json:"pty_path,omitempty"` // WebSocket path for full remote TTY
	AttachHint string `json:"attach_hint,omitempty"`
}

type CreateRequest struct {
	Agent  string `json:"agent"`
	Cwd    string `json:"cwd"`
	Name   string `json:"name,omitempty"`
	Prompt string `json:"prompt,omitempty"` // typed into TTY after start (not -p)
	Mode   Mode   `json:"mode,omitempty"`   // default tty
}

// Manager creates interactive agent sessions in tmux.
type Manager struct {
	cfg     *config.Config
	dir     string
	log     *slog.Logger
	mu      sync.Mutex
	byID    map[string]*Session
	sshHost string
}

func NewManager(cfg *config.Config, log *slog.Logger) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	dir := filepath.Join(cfg.JobsDir, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:     cfg,
		dir:     dir,
		log:     log,
		byID:    map[string]*Session{},
		sshHost: cfg.Sessions.SSHHost,
	}
	_ = m.loadAll()
	m.refreshStates()
	return m, nil
}

func (m *Manager) Create(req CreateRequest) (*Session, error) {
	if req.Agent == "" {
		req.Agent = "claude"
	}
	req.Cwd = strings.TrimSpace(req.Cwd)
	if req.Cwd == "" {
		if m.cfg.DefaultCwd != "" {
			req.Cwd = m.cfg.DefaultCwd
		} else {
			req.Cwd = "." // workspace_root
		}
	}
	if req.Mode == "" {
		req.Mode = ModeTTY
	}
	if req.Mode != ModeTTY && req.Mode != ModePrint {
		return nil, fmt.Errorf("mode must be %q or %q", ModeTTY, ModePrint)
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux required for TTY sessions: %w", err)
	}

	acfg, ok := m.cfg.Agent(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", req.Agent)
	}
	bin := acfg.Bin
	if bin == "" {
		return nil, fmt.Errorf("agent %q has empty bin", req.Agent)
	}
	resolved := resolveBin(bin)
	if resolved == "" {
		return nil, fmt.Errorf("agent binary %q not found on PATH (install it or fix [agents.%s].bin)", bin, req.Agent)
	}
	bin = resolved

	cwdAbs, err := pathallow.Resolve(m.cfg.WorkspaceRoot, req.Cwd, m.cfg.Allow.Paths)
	if err != nil {
		// retry default if client sent "." and that failed
		if (req.Cwd == "." || req.Cwd == "") && m.cfg.DefaultCwd != "" && m.cfg.DefaultCwd != req.Cwd {
			req.Cwd = m.cfg.DefaultCwd
			cwdAbs, err = pathallow.Resolve(m.cfg.WorkspaceRoot, req.Cwd, m.cfg.Allow.Paths)
		}
		if err != nil {
			return nil, err
		}
	}
	if st, err := os.Stat(cwdAbs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("cwd does not exist or is not a directory: %s", req.Cwd)
	}

	id := "s_" + ulid.Make().String()
	// tmux session names: limited charset
	tmuxName := "la-" + strings.ReplaceAll(id, "_", "")
	if len(tmuxName) > 50 {
		tmuxName = tmuxName[:50]
	}

	args := m.buildArgs(req.Agent, acfg, req.Mode, req.Prompt)
	// tmux new-session -d -s NAME -c CWD BIN ARGS...
	tmuxArgs := []string{
		"new-session", "-d",
		"-s", tmuxName,
		"-c", cwdAbs,
		"--", bin,
	}
	tmuxArgs = append(tmuxArgs, args...)

	cmd := exec.Command("tmux", tmuxArgs...)
	cmd.Env = m.sessionEnv() // full-ish env: OAuth + DISPLAY for headed browsers
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tmux new-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// For TTY mode, optional seed prompt is typed into the interactive UI (not -p).
	if req.Mode == ModeTTY && req.Prompt != "" {
		// small delay so claude can draw UI
		time.Sleep(400 * time.Millisecond)
		// send prompt then Enter — user can still edit/stop
		_ = exec.Command("tmux", "send-keys", "-t", tmuxName, "-l", req.Prompt).Run()
		_ = exec.Command("tmux", "send-keys", "-t", tmuxName, "Enter").Run()
	}

	s := &Session{
		ID:        id,
		Name:      req.Name,
		Agent:     req.Agent,
		Mode:      req.Mode,
		Cwd:       req.Cwd,
		CwdAbs:    cwdAbs,
		Tmux:      tmuxName,
		State:     StateRunning,
		Prompt:    req.Prompt,
		CreatedAt: time.Now().UTC(),
	}
	m.fillAttach(s)
	if err := m.save(s); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.byID[s.ID] = s
	m.mu.Unlock()
	m.log.Info("session started", "id", s.ID, "tmux", s.Tmux, "agent", s.Agent, "mode", s.Mode, "cwd", s.Cwd)
	return s, nil
}

func (m *Manager) buildArgs(agentName string, acfg config.AgentConfig, mode Mode, prompt string) []string {
	name := strings.ToLower(agentName)
	if mode == ModePrint {
		// explicit API/print path only
		args := append([]string{}, acfg.PrintArgs...)
		if len(args) == 0 {
			if name == "claude" {
				args = []string{"-p"}
			}
		}
		if prompt != "" {
			if name == "claude" && (hasFlag(args, "-p") || hasFlag(args, "--print")) {
				args = append(args, prompt)
			} else {
				args = append(args, prompt)
			}
		}
		return args
	}
	// TTY / interactive: use args only (default empty for claude → pure interactive)
	args := append([]string{}, acfg.Args...)
	// do NOT pass prompt as CLI arg for interactive claude — that can force print-like behavior
	// seed is sent via tmux send-keys instead
	return args
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func resolveBin(bin string) string {
	if filepath.IsAbs(bin) {
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			return bin
		}
		return ""
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		if u, err := os.UserHomeDir(); err == nil {
			home = u
		}
	}
	candidates := []string{
		filepath.Join(home, ".local", "bin"),
		"/usr/local/bin",
		filepath.Join(home, ".grok", "bin"),
		filepath.Join(home, ".opencode", "bin"),
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, bin)
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

func (m *Manager) sessionEnv() []string {
	env := os.Environ()
	home := os.Getenv("HOME")
	if home == "" {
		if u, err := os.UserHomeDir(); err == nil {
			home = u
		}
	}
	pathExtra := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".grok", "bin"),
		filepath.Join(home, ".opencode", "bin"),
		"/usr/local/bin",
	}
	var cleaned []string
	for _, p := range pathExtra {
		if p != "" && p != "/.local/bin" {
			cleaned = append(cleaned, p)
		}
	}
	pathExtra = cleaned
	pathSet := false
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			path := e[5:]
			env[i] = "PATH=" + strings.Join(pathExtra, ":") + ":" + path
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH="+strings.Join(pathExtra, ":")+":/usr/bin:/bin")
	}

	// Non-headless browser support (Playwright / Chromium need a real or virtual display)
	display := m.cfg.Sessions.Display
	if display == "" {
		display = os.Getenv("DISPLAY")
	}
	if display != "" {
		env = setOrReplaceEnv(env, "DISPLAY", display)
		// Prefer headed browsers when a display is available
		env = setOrReplaceEnv(env, "PLAYWRIGHT_HEADLESS", "0")
		env = setOrReplaceEnv(env, "HEADED", "1")
	}
	if p := m.cfg.Sessions.PlaywrightBrowsersPath; p != "" {
		env = setOrReplaceEnv(env, "PLAYWRIGHT_BROWSERS_PATH", p)
	} else if home != "" {
		// default cache used by npx playwright install
		def := filepath.Join(home, ".cache", "ms-playwright")
		if st, err := os.Stat(def); err == nil && st.IsDir() {
			env = setOrReplaceEnv(env, "PLAYWRIGHT_BROWSERS_PATH", def)
		}
	}
	if s := m.cfg.Sessions.PlaywrightServer; s != "" {
		env = setOrReplaceEnv(env, "PLAYWRIGHT_SERVER", s)
		// common convention for tools that connect to a remote browser
		env = setOrReplaceEnv(env, "PW_TEST_SERVER", s)
	}
	// Chromium/Playwright sandbox often fails as root in containers/VMs
	env = setOrReplaceEnv(env, "PLAYWRIGHT_CHROMIUM_SANDBOX", "0")

	for k, v := range m.cfg.Sessions.Env {
		if k == "" {
			continue
		}
		env = setOrReplaceEnv(env, k, v)
	}
	return env
}

func setOrReplaceEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func (m *Manager) fillAttach(s *Session) {
	s.Attach = fmt.Sprintf("tmux attach -t %s", s.Tmux)
	s.PTYPath = "/v1/sessions/" + s.ID + "/pty"
	if m.sshHost != "" {
		s.SSHAttach = fmt.Sprintf("ssh -t %s -- tmux attach -t %s", m.sshHost, s.Tmux)
	}
	s.AttachHint = "Full remote PTY: agentsctl session open " + s.ID + "  (WebSocket, no SSH). TUI: agentsctl tui"
}

func (m *Manager) Get(id string) (*Session, error) {
	m.refreshStates()
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	cp := *s
	m.fillAttach(&cp)
	return &cp, nil
}

func (m *Manager) List() ([]*Session, error) {
	m.refreshStates()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.byID))
	for _, s := range m.byID {
		cp := *s
		m.fillAttach(&cp)
		out = append(out, &cp)
	}
	// newest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (m *Manager) Kill(id string) (*Session, error) {
	s, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	_ = exec.Command("tmux", "kill-session", "-t", s.Tmux).Run()
	s.State = StateKilled
	m.mu.Lock()
	if cur, ok := m.byID[id]; ok {
		cur.State = StateKilled
		_ = m.save(cur)
	}
	m.mu.Unlock()
	m.fillAttach(s)
	return s, nil
}

// AttachCommand returns shell command to attach (local tmux).
func (m *Manager) AttachCommand(id string) (string, error) {
	s, err := m.Get(id)
	if err != nil {
		return "", err
	}
	if s.State != StateRunning {
		return "", fmt.Errorf("session not running (state=%s)", s.State)
	}
	return s.Attach, nil
}

func (m *Manager) refreshStates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.byID {
		if s.State != StateRunning {
			continue
		}
		// tmux has-session
		err := exec.Command("tmux", "has-session", "-t", s.Tmux).Run()
		if err != nil {
			s.State = StateExited
			_ = m.save(s)
		}
	}
}

func (m *Manager) path(id string) string {
	return filepath.Join(m.dir, id+".json")
}

func (m *Manager) save(s *Session) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path(s.ID), b, 0o600)
}

func (m *Manager) loadAll() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		m.byID[s.ID] = &s
	}
	return nil
}

// RunningCount for status.
func (m *Manager) RunningCount() int {
	m.refreshStates()
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.byID {
		if s.State == StateRunning {
			n++
		}
	}
	return n
}

// Prune removes non-running sessions older than maxAge (and their JSON files).
// If maxAge <= 0, removes all non-running sessions.
func (m *Manager) Prune(maxAge time.Duration) (removed int, err error) {
	m.refreshStates()
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var toDelete []string
	for id, s := range m.byID {
		if s.State == StateRunning {
			continue
		}
		if maxAge > 0 && now.Sub(s.CreatedAt) < maxAge {
			continue
		}
		toDelete = append(toDelete, id)
	}
	for _, id := range toDelete {
		delete(m.byID, id)
		_ = os.Remove(m.path(id))
		removed++
	}
	return removed, nil
}
