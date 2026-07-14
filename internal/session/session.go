package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/reloadlife/agents/internal/agentacct"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
	"github.com/reloadlife/agents/internal/workspaces"
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
	// Multi-account (cursor-account-switcher profiles)
	Account     string `json:"account,omitempty"`      // profile id e.g. personal|work
	AccountMode string `json:"account_mode,omitempty"` // isolated|global
	AccountHome string `json:"account_home,omitempty"` // isolated HOME path
	// AgentSessionID is the agent CLI's native conversation id (e.g. grok UUID).
	// Used on resume so chat history survives host reboot (grok --resume, etc.).
	// Distinct from ID (agentsd control-plane id / tmux name).
	AgentSessionID string `json:"agent_session_id,omitempty"`
	// Optional git worktree isolation (parallel agents on separate checkouts).
	Worktree     bool   `json:"worktree,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"` // relative path of worktree (same as Cwd when set)
	BaseCwd      string `json:"base_cwd,omitempty"`      // original cwd before worktree isolation
	Branch       string `json:"branch,omitempty"`        // branch checked out in the worktree
	// GitBranch is best-effort current branch for the session cwd (list/get enrichment).
	GitBranch string `json:"git_branch,omitempty"`
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
	// Account: cursor-switch profile id for this agent platform (e.g. personal, work).
	// Mode isolated (default when set): materialize auth under a private HOME so
	// multiple accounts can run in parallel. global: switch the host-wide login first.
	Account     string `json:"account,omitempty"`
	AccountMode string `json:"account_mode,omitempty"` // isolated|global
	// Worktree: when true and cwd is inside a git repo, create an isolated git
	// worktree for this session (branch agents/<short-id> or WorktreeBranch).
	Worktree       bool   `json:"worktree,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
}

// ArchiveFunc optionally archives pane data (recording store).
type ArchiveFunc func(sessionID, agent, cwd, name, reason string, data []byte)

// OnEventFunc is called for lifecycle events (notify / audit / auto-note).
type OnEventFunc func(typ, sessionID, agent, cwd, name, message string)

// Manager creates interactive agent sessions in tmux.
type Manager struct {
	cfg     *config.Config
	dir     string
	log     *slog.Logger
	mu      sync.Mutex
	byID    map[string]*Session
	sshHost string
	// optional hooks
	Archive ArchiveFunc
	OnEvent OnEventFunc
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
	// Resource limit: max concurrent running TTY sessions
	if max := m.cfg.Sessions.MaxConcurrent; max > 0 {
		m.refreshStates()
		if n := m.RunningCount(); n >= max {
			return nil, fmt.Errorf("max concurrent sessions reached (%d) — stop one first or raise sessions.max_concurrent", max)
		}
	}

	shell := IsShellAgent(req.Agent)
	var acfg config.AgentConfig
	var bin string
	var args []string
	if shell {
		// Built-in plain shell terminal — no [agents.shell] config required.
		req.Agent = "shell"
		bin = defaultShell()
		args = nil
		req.Account = "" // no multi-account for shell
	} else {
		var ok bool
		acfg, ok = m.cfg.Agent(req.Agent)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q", req.Agent)
		}
		bin = acfg.Bin
		if bin == "" {
			return nil, fmt.Errorf("agent %q has empty bin", req.Agent)
		}
		resolved := resolveBin(bin)
		if resolved == "" {
			return nil, fmt.Errorf("agent binary %q not found on PATH (install it or fix [agents.%s].bin)", bin, req.Agent)
		}
		bin = resolved
	}

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

	// Optional git worktree isolation for parallel agents.
	baseCwd := req.Cwd
	worktree := false
	worktreePath := ""
	branch := ""
	var worktreeRepoAbs string
	if req.Worktree {
		if workspaces.IsGitWorkTree(cwdAbs) {
			wt, werr := workspaces.CreateSessionWorktree(
				m.cfg.WorkspaceRoot,
				req.Cwd,
				cwdAbs,
				id,
				req.WorktreeBranch,
				m.cfg.Allow.Paths,
			)
			if werr != nil {
				return nil, fmt.Errorf("worktree: %w", werr)
			}
			worktree = true
			worktreePath = wt.Path
			baseCwd = wt.BaseCwd
			branch = wt.Branch
			worktreeRepoAbs = wt.RepoAbs
			req.Cwd = wt.Path
			cwdAbs = wt.Abs
			m.log.Info("session worktree created", "id", id, "path", wt.Path, "branch", wt.Branch, "base_cwd", wt.BaseCwd)
		} else {
			m.log.Info("worktree requested but cwd is not a git repo; continuing without isolation", "cwd", req.Cwd)
		}
	}

	// Pin a native conversation id when the CLI supports it (e.g. grok --session-id)
	// so resume after reboot can restore chat, not just a blank process.
	agentSessID := ""
	if !shell && req.Mode != ModePrint && canPinSessionID(req.Agent) {
		agentSessID = newNativeSessionID()
	}
	if !shell {
		args = m.buildArgs(req.Agent, acfg, req.Mode, req.Prompt, agentSessID, false)
	}
	env := m.sessionEnv()
	account := strings.TrimSpace(req.Account)
	accountMode := strings.ToLower(strings.TrimSpace(req.AccountMode))
	accountHome := ""
	if !shell && account != "" {
		plat := agentacct.MapAgentToPlatform(req.Agent)
		if plat == "" {
			if worktree {
				_ = workspaces.RemoveWorktree(worktreeRepoAbs, cwdAbs)
			}
			return nil, fmt.Errorf("agent %q does not support multi-account profiles (cursor/claude/codex/grok)", req.Agent)
		}
		am, err := agentacct.New(m.cfg.JobsDir)
		if err != nil {
			if worktree {
				_ = workspaces.RemoveWorktree(worktreeRepoAbs, cwdAbs)
			}
			return nil, err
		}
		if accountMode == "global" {
			if err := am.Switch(plat, account); err != nil {
				if worktree {
					_ = workspaces.RemoveWorktree(worktreeRepoAbs, cwdAbs)
				}
				return nil, fmt.Errorf("account switch (%s/%s): %w", plat, account, err)
			}
			accountMode = "global"
		} else {
			// isolated (default): parallel-safe private HOME
			home, err := am.EnsureIsolated(plat, account)
			if err != nil {
				if worktree {
					_ = workspaces.RemoveWorktree(worktreeRepoAbs, cwdAbs)
				}
				return nil, fmt.Errorf("account isolate (%s/%s): %w — save a profile first: cursor-switch --platform %s save %s", plat, account, err, plat, account)
			}
			accountHome = home
			accountMode = "isolated"
			env = agentacct.SessionEnv(env, plat, home)
		}
	}
	if err := m.startTmux(tmuxName, cwdAbs, bin, args, env); err != nil {
		if worktree {
			_ = workspaces.RemoveWorktree(worktreeRepoAbs, cwdAbs)
		}
		return nil, err
	}

	// For TTY mode, optional seed prompt is typed into the interactive UI (not -p).
	// Skip for plain shell terminals.
	if !shell && req.Mode == ModeTTY && req.Prompt != "" {
		m.seedPrompt(tmuxName, req.Prompt)
	}

	s := &Session{
		ID:             id,
		Name:           req.Name,
		Agent:          req.Agent,
		Mode:           req.Mode,
		Cwd:            req.Cwd,
		CwdAbs:         cwdAbs,
		Tmux:           tmuxName,
		State:          StateRunning,
		Prompt:         req.Prompt,
		CreatedAt:      time.Now().UTC(),
		Account:        account,
		AccountMode:    accountMode,
		AccountHome:    accountHome,
		AgentSessionID: agentSessID,
	}
	if worktree {
		s.Worktree = true
		s.WorktreePath = worktreePath
		s.BaseCwd = baseCwd
		s.Branch = branch
	}
	m.fillAttach(s)
	if err := m.save(s); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.byID[s.ID] = s
	m.mu.Unlock()
	m.log.Info("session started", "id", s.ID, "tmux", s.Tmux, "agent", s.Agent, "mode", s.Mode, "cwd", s.Cwd, "worktree", worktree, "account", account, "account_mode", accountMode)
	if m.OnEvent != nil {
		m.OnEvent("session.started", s.ID, s.Agent, s.Cwd, s.Name, "session started")
	}
	return s, nil
}

// Default pane scrollback kept inside tmux (and mirrored into xterm on attach).
const defaultHistoryLimit = 50000

// startTmux creates a detached tmux session running bin+args in cwdAbs.
// Setsid so a terminal SIGHUP/process-group kill of agentsd does not take
// down the agent. Under systemd, also set KillMode=process (see deploy units)
// so control-group stop does not kill the tmux server in the same cgroup.
func (m *Manager) startTmux(tmuxName, cwdAbs, bin string, args []string, env []string) error {
	tmuxArgs := []string{
		"new-session", "-d",
		"-s", tmuxName,
		"-c", cwdAbs,
		"--", bin,
	}
	tmuxArgs = append(tmuxArgs, args...)
	cmd := exec.Command("tmux", tmuxArgs...)
	if env == nil {
		env = m.sessionEnv()
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Large scrollback so re-attach can dump history (tmux default is often 2000).
	_ = exec.Command("tmux", "set-option", "-t", tmuxName, "history-limit", fmt.Sprintf("%d", defaultHistoryLimit)).Run()
	return nil
}

func (m *Manager) historyPath(id string) string {
	return filepath.Join(m.dir, id+".pane")
}

// capturePane dumps tmux scrollback + visible pane with ANSI colors.
// -S - = from start of history; -e = escape sequences; -p = stdout.
func capturePane(tmuxName string) ([]byte, error) {
	out, err := exec.Command(
		"tmux", "capture-pane",
		"-t", tmuxName,
		"-e", // keep ANSI colors
		"-p", // print to stdout
		"-S", "-", // from beginning of history
	).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SnapshotHistory captures live pane output to disk (best-effort).
// Called before kill and on PTY detach so dead sessions keep a transcript.
// reason is for optional archive (kill|detach|manual).
func (m *Manager) SnapshotHistory(id string) {
	m.SnapshotHistoryReason(id, "detach")
}

// SnapshotHistoryReason is like SnapshotHistory but tags the archive reason.
func (m *Manager) SnapshotHistoryReason(id, reason string) {
	m.mu.Lock()
	s, ok := m.byID[id]
	var tmux string
	var agent, cwd, name string
	if ok {
		tmux = s.Tmux
		agent, cwd, name = s.Agent, s.Cwd, s.Name
	}
	m.mu.Unlock()
	if tmux == "" || !tmuxAlive(tmux) {
		return
	}
	data, err := capturePane(tmux)
	if err != nil || len(data) == 0 {
		return
	}
	_ = os.WriteFile(m.historyPath(id), data, 0o600)
	if m.Archive != nil && m.cfg != nil && m.cfg.RecordingEnabled() {
		m.Archive(id, agent, cwd, name, reason, data)
	}
}

// History returns the best available scrollback for a session:
// live tmux capture if running, else last on-disk snapshot.
func (m *Manager) History(id string) (data []byte, source string, err error) {
	m.refreshStates()
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return nil, "", fmt.Errorf("session not found")
	}
	tmux := s.Tmux
	m.mu.Unlock()

	if tmuxAlive(tmux) {
		data, err = capturePane(tmux)
		if err == nil && len(data) > 0 {
			// refresh disk snapshot while we're here
			_ = os.WriteFile(m.historyPath(id), data, 0o600)
			return data, "live", nil
		}
	}
	data, err = os.ReadFile(m.historyPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("no history snapshot for session")
		}
		return nil, "", err
	}
	return data, "snapshot", nil
}

func (m *Manager) seedPrompt(tmuxName, prompt string) {
	// Longer wait for slow agent CLIs; orientation seeds are more useful after UI is up.
	time.Sleep(900 * time.Millisecond)
	// Chunk large seeds — some tmux builds choke on very long -l strings.
	const chunk = 1200
	for i := 0; i < len(prompt); i += chunk {
		end := i + chunk
		if end > len(prompt) {
			end = len(prompt)
		}
		_ = exec.Command("tmux", "send-keys", "-t", tmuxName, "-l", prompt[i:end]).Run()
		if end < len(prompt) {
			time.Sleep(40 * time.Millisecond)
		}
	}
	_ = exec.Command("tmux", "send-keys", "-t", tmuxName, "Enter").Run()
}

func tmuxAlive(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// buildArgs constructs CLI args for print jobs or interactive TTY.
// nativeID is the agent-native conversation id; resume selects resume/continue flags.
func (m *Manager) buildArgs(agentName string, acfg config.AgentConfig, mode Mode, prompt, nativeID string, resume bool) []string {
	name := strings.ToLower(agentName)
	if mode == ModePrint {
		// explicit API/print path only — no chat resume
		args := append([]string{}, acfg.PrintArgs...)
		if len(args) == 0 {
			if name == "claude" || strings.Contains(name, "claude") {
				args = []string{"-p"}
			}
		}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args
	}
	// TTY / interactive: seed prompt via tmux send-keys, not CLI args.
	// Resume/create flags restore agent chat across reboots when possible.
	return ttyLaunchArgs(agentName, acfg, nativeID, resume)
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

// gitBranchTimeout caps best-effort branch lookups so list endpoints stay snappy.
const gitBranchTimeout = 400 * time.Millisecond

// gitBranch returns the current branch for cwdAbs (or "" on any error / non-repo).
// Detached HEAD is reported as "HEAD".
func gitBranch(cwdAbs string) string {
	cwdAbs = strings.TrimSpace(cwdAbs)
	if cwdAbs == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitBranchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", cwdAbs, "rev-parse", "--abbrev-ref", "HEAD")
	// Avoid inheriting a huge env; git only needs PATH for helpers.
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sessionCwdAbs(s *Session) string {
	if s == nil {
		return ""
	}
	if strings.TrimSpace(s.CwdAbs) != "" {
		return s.CwdAbs
	}
	return strings.TrimSpace(s.Cwd)
}

// enrichGitBranch fills GitBranch best-effort. branchCache dedupes by cwd within one list call.
func enrichGitBranch(s *Session, branchCache map[string]string) {
	if s == nil {
		return
	}
	abs := sessionCwdAbs(s)
	if abs == "" {
		s.GitBranch = ""
		return
	}
	if branchCache != nil {
		if b, ok := branchCache[abs]; ok {
			s.GitBranch = b
			return
		}
	}
	b := gitBranch(abs)
	if branchCache != nil {
		branchCache[abs] = b
	}
	s.GitBranch = b
}

func (m *Manager) Get(id string) (*Session, error) {
	m.refreshStates()
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session not found")
	}
	cp := *s
	m.fillAttach(&cp)
	m.mu.Unlock()
	// Best-effort git outside the lock so a slow rev-parse never blocks other ops.
	enrichGitBranch(&cp, nil)
	return &cp, nil
}

func (m *Manager) List() ([]*Session, error) {
	m.refreshStates()
	m.mu.Lock()
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
	m.mu.Unlock()
	// One git lookup per unique cwd (session lists are small).
	branchCache := make(map[string]string, 8)
	for _, s := range out {
		enrichGitBranch(s, branchCache)
	}
	return out, nil
}

func (m *Manager) Kill(id string) (*Session, error) {
	s, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	// Snapshot scrollback before destroying tmux so resume/history still work.
	m.SnapshotHistoryReason(id, "kill")
	_ = exec.Command("tmux", "kill-session", "-t", s.Tmux).Run()
	s.State = StateKilled
	m.mu.Lock()
	if cur, ok := m.byID[id]; ok {
		cur.State = StateKilled
		_ = m.save(cur)
	}
	m.mu.Unlock()
	m.fillAttach(s)
	if m.OnEvent != nil {
		m.OnEvent("session.stopped", s.ID, s.Agent, s.Cwd, s.Name, "session stopped/killed")
	}
	return s, nil
}

// Delete stops the agent if still running and removes session metadata (JSON +
// pane snapshot) so it no longer appears in list. Irreversible.
// When the session used a git worktree, best-effort removes that worktree
// (never fails Delete if remove fails; never removes the main repo).
func (m *Manager) Delete(id string) error {
	m.refreshStates()
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session not found")
	}
	tmux := s.Tmux
	wt := s.Worktree
	wtAbs := s.CwdAbs
	baseCwd := s.BaseCwd
	m.mu.Unlock()

	// Best-effort history snapshot then kill tmux (running or not).
	agent, cwd, name := s.Agent, s.Cwd, s.Name
	if tmuxAlive(tmux) {
		m.SnapshotHistoryReason(id, "delete")
		_ = exec.Command("tmux", "kill-session", "-t", tmux).Run()
	}

	if wt && wtAbs != "" {
		m.cleanupWorktree(baseCwd, wtAbs)
	}

	m.mu.Lock()
	delete(m.byID, id)
	m.mu.Unlock()
	_ = os.Remove(m.path(id))
	_ = os.Remove(m.historyPath(id))
	m.log.Info("session deleted", "id", id, "tmux", tmux, "worktree", wt)
	if m.OnEvent != nil {
		m.OnEvent("session.deleted", id, agent, cwd, name, "session deleted")
	}
	return nil
}

// cleanupWorktree best-effort removes a session-owned git worktree.
func (m *Manager) cleanupWorktree(baseCwdRel, worktreeAbs string) {
	repoAbs := ""
	if baseCwdRel != "" {
		if abs, err := pathallow.Resolve(m.cfg.WorkspaceRoot, baseCwdRel, m.cfg.Allow.Paths); err == nil {
			if root, rerr := workspaces.RepoRoot(abs); rerr == nil {
				repoAbs = root
			}
		}
	}
	// When repoAbs is empty, RemoveWorktree discovers the common dir from the worktree.
	if err := workspaces.RemoveWorktree(repoAbs, worktreeAbs); err != nil {
		m.log.Warn("worktree remove failed (ignored)", "path", worktreeAbs, "err", err)
	} else {
		m.log.Info("session worktree removed", "path", worktreeAbs)
	}
}

// Resume re-opens a session that still has metadata on disk.
//
//   - If the tmux session is still alive (e.g. agentsd restarted but agents
//     survived): mark running and return — client can attach as usual.
//   - If the agent process is gone (host reboot, kill, cgroup wipe): start a
//     new tmux session and ask the agent CLI to restore its conversation
//     (e.g. grok --resume <id>, claude --resume <id>, codex resume <id>).
//     Terminal scrollback is still restored from the pane snapshot on PTY attach.
func (m *Manager) Resume(id string) (*Session, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux required for TTY sessions: %w", err)
	}

	m.refreshStates()
	m.mu.Lock()
	cur, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session not found")
	}
	// snapshot under lock
	s := *cur
	m.mu.Unlock()

	if tmuxAlive(s.Tmux) {
		m.mu.Lock()
		if live, ok := m.byID[id]; ok {
			live.State = StateRunning
			_ = m.save(live)
			s = *live
		}
		m.mu.Unlock()
		m.fillAttach(&s)
		m.log.Info("session resume: already live", "id", s.ID, "tmux", s.Tmux)
		return &s, nil
	}

	shell := IsShellAgent(s.Agent)
	var acfg config.AgentConfig
	var bin string
	var args []string
	if shell {
		bin = defaultShell()
		args = nil
	} else {
		var ok bool
		acfg, ok = m.cfg.Agent(s.Agent)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (configure [agents.%s] or start a new session)", s.Agent, s.Agent)
		}
		bin = acfg.Bin
		if bin == "" {
			return nil, fmt.Errorf("agent %q has empty bin", s.Agent)
		}
		resolved := resolveBin(bin)
		if resolved == "" {
			return nil, fmt.Errorf("agent binary %q not found on PATH", bin)
		}
		bin = resolved
	}

	// Re-resolve cwd (allowlist may have changed; path must still exist).
	cwdAbs, err := pathallow.Resolve(m.cfg.WorkspaceRoot, s.Cwd, m.cfg.Allow.Paths)
	if err != nil {
		// fall back to stored absolute path if still valid and under root
		if s.CwdAbs != "" {
			if st, stErr := os.Stat(s.CwdAbs); stErr == nil && st.IsDir() {
				cwdAbs = s.CwdAbs
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if st, err := os.Stat(cwdAbs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("cwd does not exist or is not a directory: %s", s.Cwd)
	}
	s.CwdAbs = cwdAbs

	mode := s.Mode
	if mode == "" {
		mode = ModeTTY
	}

	// Resolve native agent conversation id for chat restore.
	nativeID := strings.TrimSpace(s.AgentSessionID)
	if !shell {
		if nativeID == "" {
			if found := discoverNativeSessionID(&s); found != "" {
				nativeID = found
				m.log.Info("session resume: discovered agent conversation", "id", s.ID, "agent_session_id", nativeID)
			}
		}
		args = m.buildArgs(s.Agent, acfg, mode, "", nativeID, true)
	}
	env := m.sessionEnv()
	if !shell && s.Account != "" && s.AccountMode != "global" {
		plat := agentacct.MapAgentToPlatform(s.Agent)
		if plat != "" {
			if am, err := agentacct.New(m.cfg.JobsDir); err == nil {
				home := s.AccountHome
				if home == "" {
					home = am.IsolatedHome(plat, s.Account)
				}
				if _, err := am.Materialize(plat, s.Account, home); err == nil {
					env = agentacct.SessionEnv(env, plat, home)
					s.AccountHome = home
				}
			}
		}
	}
	// Drop any leftover name collision, then start with resume flags.
	_ = exec.Command("tmux", "kill-session", "-t", s.Tmux).Run()
	if err := m.startTmux(s.Tmux, cwdAbs, bin, args, env); err != nil {
		return nil, err
	}

	m.mu.Lock()
	live, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		_ = exec.Command("tmux", "kill-session", "-t", s.Tmux).Run()
		return nil, fmt.Errorf("session not found")
	}
	live.State = StateRunning
	live.CwdAbs = cwdAbs
	live.Mode = mode
	if nativeID != "" && live.AgentSessionID == "" {
		live.AgentSessionID = nativeID
	}
	if s.AccountHome != "" {
		live.AccountHome = s.AccountHome
	}
	_ = m.save(live)
	out := *live
	m.mu.Unlock()
	m.fillAttach(&out)
	m.log.Info("session resumed",
		"id", out.ID,
		"tmux", out.Tmux,
		"agent", out.Agent,
		"cwd", out.Cwd,
		"agent_session_id", out.AgentSessionID,
		"args", strings.Join(args, " "),
	)
	if m.OnEvent != nil {
		m.OnEvent("session.resumed", out.ID, out.Agent, out.Cwd, out.Name, "session resumed")
	}
	return &out, nil
}

// AttachCommand returns shell command to attach (local tmux).
func (m *Manager) AttachCommand(id string) (string, error) {
	s, err := m.Get(id)
	if err != nil {
		return "", err
	}
	if s.State != StateRunning {
		return "", fmt.Errorf("session not running (state=%s) — try: agentsctl session resume %s", s.State, id)
	}
	return s.Attach, nil
}

func (m *Manager) refreshStates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.byID {
		alive := tmuxAlive(s.Tmux)
		switch {
		case alive && s.State != StateRunning:
			// agentsd restarted (or metadata lag) while tmux survived
			s.State = StateRunning
			_ = m.save(s)
		case !alive && s.State == StateRunning:
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
		_ = os.Remove(m.historyPath(id))
		removed++
	}
	return removed, nil
}
