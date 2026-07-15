package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- messages from async cmds ----

type sessionsMsg struct {
	items []sessionItem
	err   error
}
type agentsMsg struct {
	names []string
	err   error
}
type workspacesMsg struct {
	paths   []string
	defPath string
	err     error
}
type createdMsg struct {
	id  string
	err error
}
type statusMsg struct {
	text string
	err  error
}
type killedMsg struct {
	err error
}
type deletedMsg struct {
	err error
}
type resumedMsg struct {
	id    string
	state string
	err   error
}
type previewMsg struct {
	id   string
	text string
	err  error
}

// ---- HTTP ----

func apiGet(cfg Config, p string, out any) error {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.BaseURL, "/")+p, nil)
	if err != nil {
		return err
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

func apiPost(cfg Config, p string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+p, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

// ---- fetch cmds ----

func fetchSessions(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Sessions []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Agent     string `json:"agent"`
				Cwd       string `json:"cwd"`
				State     string `json:"state"`
				Tmux      string `json:"tmux"`
				Branch    string `json:"branch"`
				GitBranch string `json:"git_branch"`
				Worktree  bool   `json:"worktree"`
			} `json:"sessions"`
		}
		if err := apiGet(cfg, "/v1/sessions", &out); err != nil {
			return sessionsMsg{err: err}
		}
		items := make([]sessionItem, 0, len(out.Sessions))
		for _, s := range out.Sessions {
			br := s.GitBranch
			if br == "" {
				br = s.Branch
			}
			items = append(items, sessionItem{
				id:       s.ID,
				name:     s.Name,
				agent:    s.Agent,
				cwd:      s.Cwd,
				state:    s.State,
				tmux:     s.Tmux,
				branch:   br,
				worktree: s.Worktree,
			})
		}
		return sessionsMsg{items: items}
	}
}

func fetchAgents(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Available []string `json:"available"`
		}
		if err := apiGet(cfg, "/v1/agents", &out); err != nil {
			return agentsMsg{names: []string{"claude", "grok", "codex", "opencode", "cursor"}}
		}
		if len(out.Available) == 0 {
			return agentsMsg{names: []string{"claude", "grok", "codex", "opencode", "cursor"}}
		}
		return agentsMsg{names: out.Available}
	}
}

func fetchWorkspaces(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Default    string `json:"default_cwd"`
			Workspaces []struct {
				Path    string `json:"path"`
				Default bool   `json:"default"`
			} `json:"workspaces"`
		}
		if err := apiGet(cfg, "/v1/workspaces", &out); err != nil {
			return workspacesMsg{paths: []string{cfg.DefaultCwd}, defPath: cfg.DefaultCwd}
		}
		paths := make([]string, 0, len(out.Workspaces))
		def := out.Default
		if def == "" {
			def = cfg.DefaultCwd
		}
		for _, w := range out.Workspaces {
			paths = append(paths, w.Path)
			if w.Default {
				def = w.Path
			}
		}
		if len(paths) == 0 {
			paths = []string{"."}
		}
		return workspacesMsg{paths: paths, defPath: def}
	}
}

func createSession(cfg Config, cwd, agent string, worktree bool) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{"agent": agent, "cwd": cwd, "mode": "tty"}
		if worktree {
			body["worktree"] = true
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := apiPost(cfg, "/v1/sessions", body, &out); err != nil {
			return createdMsg{err: err}
		}
		return createdMsg{id: out.ID}
	}
}

func killSession(cfg Config, id string) tea.Cmd {
	return func() tea.Msg {
		if err := apiPost(cfg, "/v1/sessions/"+id+"/kill", map[string]any{}, nil); err != nil {
			return killedMsg{err: err}
		}
		return killedMsg{}
	}
}

func deleteSession(cfg Config, id string) tea.Cmd {
	return func() tea.Msg {
		// POST /delete is the portable alias used by agentsctl
		if err := apiPost(cfg, "/v1/sessions/"+id+"/delete", map[string]any{}, nil); err != nil {
			return deletedMsg{err: err}
		}
		return deletedMsg{}
	}
}

func resumeSession(cfg Config, id string) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			ID    string `json:"id"`
			State string `json:"state"`
		}
		if err := apiPost(cfg, "/v1/sessions/"+id+"/resume", map[string]any{}, &out); err != nil {
			return resumedMsg{id: id, err: err}
		}
		if out.ID == "" {
			out.ID = id
		}
		return resumedMsg{id: out.ID, state: out.State}
	}
}

func fetchStatus(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var raw map[string]any
		if err := apiGet(cfg, "/v1/status", &raw); err != nil {
			return statusMsg{err: err}
		}
		return statusMsg{text: formatStatusPanel(raw)}
	}
}

func fetchPreview(cfg Config, id string) tea.Cmd {
	return func() tea.Msg {
		if id == "" {
			return previewMsg{id: id, text: ""}
		}
		// Prefer dedicated preview if present; fall back to history JSON.
		var text string
		var previewOut struct {
			Text  string   `json:"text"`
			Lines []string `json:"lines"`
		}
		if err := apiGet(cfg, "/v1/sessions/"+id+"/preview", &previewOut); err == nil {
			if previewOut.Text != "" {
				text = previewOut.Text
			} else if len(previewOut.Lines) > 0 {
				text = strings.Join(previewOut.Lines, "\n")
			}
		}
		if text == "" {
			var hist struct {
				Text string `json:"text"`
			}
			if err := apiGet(cfg, "/v1/sessions/"+id+"/history?format=json", &hist); err != nil {
				return previewMsg{id: id, text: "(no preview)"}
			}
			text = hist.Text
		}
		return previewMsg{id: id, text: tailPreview(text, 12)}
	}
}

// crude ANSI / OSC strip for scrollback preview
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\r`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func tailPreview(raw string, n int) string {
	s := stripANSI(raw)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		if len(ln) > 120 {
			ln = ln[:117] + "…"
		}
		lines[i] = ln
	}
	if len(lines) == 0 {
		return "(empty scrollback)"
	}
	return strings.Join(lines, "\n")
}

func formatStatusPanel(raw map[string]any) string {
	host, _ := raw["host"].(string)
	goos, _ := raw["goos"].(string)
	arch, _ := raw["goarch"].(string)
	cpus := asInt(raw["num_cpu"])
	uptime := formatUptime(asFloat(raw["uptime_sec"]))
	tty := asInt(raw["tty_sessions"])
	docker, _ := raw["docker"].(string)
	display, _ := raw["display"].(string)
	displayOK, _ := raw["display_ok"].(string)
	when, _ := raw["time"].(string)
	if t, err := time.Parse(time.RFC3339Nano, when); err == nil {
		when = t.Local().Format("15:04:05")
	}

	load := formatLoad(raw["load"])
	mem := formatMemBrief(raw["mem"])
	disk := formatDiskBrief(raw["disk"])
	jobs := formatJobsBrief(raw["jobs"])
	agents := formatAgentsBrief(raw["agents"])

	var b strings.Builder
	b.WriteString(titleStyle.Render(" host status ") + helpStyle.Render(when) + "\n")
	b.WriteString(fmt.Sprintf("  host     %s  %s/%s  %d cpu\n", host, goos, arch, cpus))
	b.WriteString(fmt.Sprintf("  uptime   %s\n", uptime))
	b.WriteString(fmt.Sprintf("  load     %s\n", load))
	b.WriteString(fmt.Sprintf("  memory   %s\n", mem))
	b.WriteString(fmt.Sprintf("  disk     %s\n", disk))
	b.WriteString(fmt.Sprintf("  docker   %s\n", orDash(docker)))
	b.WriteString(fmt.Sprintf("  display  %s (%s)\n", orDash(display), orDash(displayOK)))
	b.WriteString(fmt.Sprintf("  jobs     %s\n", jobs))
	b.WriteString(fmt.Sprintf("  tty      %d session(s)\n", tty))
	b.WriteString(fmt.Sprintf("  agents   %s\n", agents))
	b.WriteString(helpStyle.Render("\n  press s or esc to close"))
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func formatUptime(sec float64) string {
	if sec <= 0 {
		return "—"
	}
	d := time.Duration(sec) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 48 {
		return fmt.Sprintf("%dd %dh", h/24, h%24)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func formatLoad(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(arr))
	for _, x := range arr {
		parts = append(parts, fmt.Sprintf("%.2f", asFloat(x)))
	}
	return strings.Join(parts, "  ")
}

func formatMemBrief(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "—"
	}
	total := asInt(m["host_total_mb"])
	avail := asInt(m["host_available_mb"])
	alloc := asInt(m["alloc_mb"])
	if total > 0 {
		return fmt.Sprintf("host %d/%d MB free · daemon %d MB", avail, total, alloc)
	}
	return fmt.Sprintf("daemon %d MB", alloc)
}

func formatDiskBrief(v any) string {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return "—"
	}
	if raw, _ := m["raw"].(string); raw != "" {
		return strings.TrimSpace(raw)
	}
	p, _ := m["path"].(string)
	pct := asFloat(m["used_pct"])
	if p != "" {
		return fmt.Sprintf("%s %.0f%% used", path.Base(p), pct)
	}
	return "—"
}

func formatJobsBrief(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("running %d · queued %d", asInt(m["running"]), asInt(m["queued"]))
}

func formatAgentsBrief(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return "—"
	}
	var parts []string
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name == "mock" || name == "cursor-agent" {
			continue
		}
		if b, _ := m["available"].(bool); b {
			parts = append(parts, name)
		} else {
			parts = append(parts, name+"✗")
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "  ")
}
