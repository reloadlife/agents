package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/reloadlife/agents/internal/clientpty"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

// Config for the TUI client.
type Config struct {
	BaseURL      string
	Token        string
	DefaultAgent string
	DefaultCwd   string
}

type sessionItem struct {
	id, agent, cwd, state, tmux string
}

func (s sessionItem) Title() string { return fmt.Sprintf("%s  [%s]", s.id, s.state) }
func (s sessionItem) Description() string {
	return fmt.Sprintf("%s · %s · %s", s.agent, s.cwd, s.tmux)
}
func (s sessionItem) FilterValue() string { return s.id + " " + s.cwd + " " + s.agent }

type model struct {
	cfg          Config
	list         list.Model
	spin         spinner.Model
	status       string
	err          string
	loading      bool
	attachID     string
	quitting     bool
	agents       []string // available TTY agents
	agentIdx     int
	currentAgent string
	workspaces   []string // allowlisted cwd paths
	wsIdx        int
	currentCwd   string
}

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

func New(cfg Config) model {
	if cfg.DefaultAgent == "" {
		cfg.DefaultAgent = "claude"
	}
	if cfg.DefaultCwd == "" {
		cfg.DefaultCwd = "." // workspace root (~/workspace on agents)
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "agents · remote TTY"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	return model{
		cfg:          cfg,
		list:         l,
		spin:         sp,
		loading:      true,
		status:       "loading…",
		currentAgent: cfg.DefaultAgent,
		currentCwd:   cfg.DefaultCwd,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchSessions(m.cfg), fetchAgents(m.cfg), fetchWorkspaces(m.cfg))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.list.SetSize(msg.Width-2, h)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case sessionsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.status = "error"
			return m, nil
		}
		m.err = ""
		items := make([]list.Item, len(msg.items))
		for i := range msg.items {
			items[i] = msg.items[i]
		}
		m.list.SetItems(items)
		m.status = fmt.Sprintf("%d sessions · agent=%s · cwd=%s", len(msg.items), m.currentAgent, m.currentCwd)
		return m, nil

	case agentsMsg:
		if msg.err != nil {
			// keep defaults
			return m, nil
		}
		m.agents = msg.names
		// snap current agent index
		for i, n := range m.agents {
			if n == m.currentAgent {
				m.agentIdx = i
				break
			}
		}
		if len(m.agents) > 0 {
			// if default missing, use first available
			found := false
			for _, n := range m.agents {
				if n == m.currentAgent {
					found = true
					break
				}
			}
			if !found {
				m.currentAgent = m.agents[0]
				m.agentIdx = 0
			}
		}
		return m, nil

	case workspacesMsg:
		if msg.err != nil {
			return m, nil
		}
		m.workspaces = msg.paths
		if len(m.workspaces) == 0 {
			return m, nil
		}
		// prefer cfg default / server default if present
		want := m.currentCwd
		if want == "" || want == "." {
			if msg.defPath != "" {
				want = msg.defPath
			}
		}
		found := false
		for i, p := range m.workspaces {
			if p == want {
				m.wsIdx = i
				m.currentCwd = p
				found = true
				break
			}
		}
		if !found {
			// keep current if it was explicit; else first workspace
			if m.currentCwd == "" || m.currentCwd == "." {
				m.wsIdx = 0
				m.currentCwd = m.workspaces[0]
			}
		}
		m.status = fmt.Sprintf("cwd → %s", m.currentCwd)
		return m, nil

	case createdMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.attachID = msg.id
		m.quitting = true
		return m, tea.Quit

	case killedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		return m, fetchSessions(m.cfg)

	case statusMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.status = msg.text
			m.err = ""
		}
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter", "o":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				if it.state != "running" {
					m.err = "session not running"
					return m, nil
				}
				m.attachID = it.id
				m.quitting = true
				return m, tea.Quit
			}
		case "n":
			m.loading = true
			m.status = fmt.Sprintf("creating %s in %s…", m.currentAgent, m.currentCwd)
			return m, createSession(m.cfg, m.currentCwd, m.currentAgent)
		case "a", "tab":
			// cycle agent
			if len(m.agents) == 0 {
				m.err = "no agents loaded yet"
				return m, fetchAgents(m.cfg)
			}
			m.agentIdx = (m.agentIdx + 1) % len(m.agents)
			m.currentAgent = m.agents[m.agentIdx]
			m.status = fmt.Sprintf("agent → %s  (press n to start)", m.currentAgent)
			m.err = ""
			return m, nil
		case "w", "W":
			// cycle workspace / cwd
			if len(m.workspaces) == 0 {
				m.err = "no workspaces loaded yet"
				return m, fetchWorkspaces(m.cfg)
			}
			m.wsIdx = (m.wsIdx + 1) % len(m.workspaces)
			m.currentCwd = m.workspaces[m.wsIdx]
			m.status = fmt.Sprintf("cwd → %s  (press n to start)", m.currentCwd)
			m.err = ""
			return m, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// quick-pick agent by number and start
			idx := int(msg.String()[0] - '1')
			if len(m.agents) == 0 {
				return m, fetchAgents(m.cfg)
			}
			if idx >= 0 && idx < len(m.agents) {
				m.agentIdx = idx
				m.currentAgent = m.agents[idx]
				m.loading = true
				m.status = fmt.Sprintf("creating %s in %s…", m.currentAgent, m.currentCwd)
				return m, createSession(m.cfg, m.currentCwd, m.currentAgent)
			}
		case "r":
			m.loading = true
			return m, tea.Batch(fetchSessions(m.cfg), fetchAgents(m.cfg), fetchWorkspaces(m.cfg))
		case "x", "d":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				m.loading = true
				return m, killSession(m.cfg, it.id)
			}
		case "s":
			return m, fetchStatus(m.cfg)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting && m.attachID == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(" agents ") + helpStyle.Render(" full PTY · no SSH ") + "\n")
	agentLine := "agent: " + okStyle.Render(m.currentAgent)
	if len(m.agents) > 0 {
		var parts []string
		for i, a := range m.agents {
			label := fmt.Sprintf("%d:%s", i+1, a)
			if a == m.currentAgent {
				label = okStyle.Render(label)
			}
			parts = append(parts, label)
		}
		agentLine += "  " + helpStyle.Render("["+strings.Join(parts, " ")+"]")
	}
	b.WriteString(agentLine + "\n")
	cwdLine := "cwd:   " + okStyle.Render(m.currentCwd)
	if len(m.workspaces) > 1 {
		// show a short neighbour preview
		prev := m.workspaces[(m.wsIdx-1+len(m.workspaces))%len(m.workspaces)]
		next := m.workspaces[(m.wsIdx+1)%len(m.workspaces)]
		cwdLine += "  " + helpStyle.Render(fmt.Sprintf("(w cycle · %d · …%s | %s…)", len(m.workspaces), shortPath(prev), shortPath(next)))
	} else if len(m.workspaces) == 1 {
		cwdLine += "  " + helpStyle.Render("(1 workspace)")
	}
	b.WriteString(cwdLine + "\n")
	if m.loading {
		b.WriteString(m.spin.View() + " " + m.status + "\n")
	} else {
		b.WriteString(helpStyle.Render(m.status) + "\n")
	}
	if m.err != "" {
		b.WriteString(errStyle.Render("✗ "+m.err) + "\n")
	}
	b.WriteString(m.list.View())
	b.WriteString("\n" + helpStyle.Render("enter open · n new · a/tab agent · w cwd · 1-9 quick start · x kill · r refresh · q quit"))
	return b.String()
}

func shortPath(p string) string {
	if len(p) <= 18 {
		return p
	}
	return "…" + p[len(p)-16:]
}

// Run launches the session picker TUI; on selection, attaches full remote PTY.
func Run(cfg Config) error {
	m := New(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	fm, ok := final.(model)
	if !ok || fm.attachID == "" {
		return nil
	}
	fmt.Fprintln(os.Stderr, okStyle.Render("→ PTY attach"), fm.attachID)
	return clientpty.Attach(cfg.BaseURL, cfg.Token, fm.attachID)
}

func fetchSessions(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Sessions []struct {
				ID    string `json:"id"`
				Agent string `json:"agent"`
				Cwd   string `json:"cwd"`
				State string `json:"state"`
				Tmux  string `json:"tmux"`
			} `json:"sessions"`
		}
		if err := apiGet(cfg, "/v1/sessions", &out); err != nil {
			return sessionsMsg{err: err}
		}
		items := make([]sessionItem, 0, len(out.Sessions))
		for _, s := range out.Sessions {
			items = append(items, sessionItem{id: s.ID, agent: s.Agent, cwd: s.Cwd, state: s.State, tmux: s.Tmux})
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
			// fallback fixed list
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
			// fallback: only configured default
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

func createSession(cfg Config, cwd, agent string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{"agent": agent, "cwd": cwd, "mode": "tty"}
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

func fetchStatus(cfg Config) tea.Cmd {
	return func() tea.Msg {
		var raw map[string]any
		if err := apiGet(cfg, "/v1/status", &raw); err != nil {
			return statusMsg{err: err}
		}
		host, _ := raw["host"].(string)
		return statusMsg{text: fmt.Sprintf("host=%s · %s", host, time.Now().Format("15:04:05"))}
	}
}

func apiGet(cfg Config, path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.BaseURL, "/")+path, nil)
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

func apiPost(cfg Config, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+path, rdr)
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
