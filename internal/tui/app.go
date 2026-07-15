package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/reloadlife/agents/internal/clientpty"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	previewStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(lipgloss.Color("238")).PaddingTop(0)
	overlayStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
)

// Config for the TUI client.
type Config struct {
	BaseURL      string
	Token        string
	DefaultAgent string
	DefaultCwd   string
}

// persisted prefs across PTY detach → picker re-entry
type prefs struct {
	agent, cwd string
	worktree   bool
	filterCwd  bool
	wsIdx      int
	agentIdx   int
}

type model struct {
	cfg          Config
	list         list.Model
	spin         spinner.Model
	status       string
	err          string
	loading      bool
	attachID     string
	quitting     bool
	agents       []string
	agentIdx     int
	currentAgent string
	workspaces   []string
	wsIdx        int
	currentCwd   string
	formWorktree bool
	showHelp     bool
	showStatus   bool
	statusPanel  string
	preview      string
	previewID    string
	filterCwd    bool
	allItems     []sessionItem
	width        int
	height       int
}

func New(cfg Config) model {
	return newWithPrefs(cfg, prefs{worktree: true})
}

func newWithPrefs(cfg Config, p prefs) model {
	if cfg.DefaultAgent == "" {
		cfg.DefaultAgent = "claude"
	}
	if cfg.DefaultCwd == "" {
		cfg.DefaultCwd = "."
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "sessions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle
	l.SetShowHelp(false) // we render our own footer / ? overlay

	agent := cfg.DefaultAgent
	if p.agent != "" {
		agent = p.agent
	}
	cwd := cfg.DefaultCwd
	if p.cwd != "" {
		cwd = p.cwd
	}

	return model{
		cfg:          cfg,
		list:         l,
		spin:         sp,
		loading:      true,
		status:       "loading…",
		currentAgent: agent,
		currentCwd:   cwd,
		agentIdx:     p.agentIdx,
		wsIdx:        p.wsIdx,
		formWorktree: p.worktree, // default true from New / Run
		filterCwd:    p.filterCwd,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchSessions(m.cfg), fetchAgents(m.cfg), fetchWorkspaces(m.cfg))
}

func (m *model) setListItems(items []sessionItem) {
	m.allItems = items
	filtered := m.applyFilter(items)
	listItems := make([]list.Item, len(filtered))
	for i := range filtered {
		listItems[i] = filtered[i]
	}
	m.list.SetItems(listItems)
}

func (m model) applyFilter(items []sessionItem) []sessionItem {
	if !m.filterCwd || m.currentCwd == "" || m.currentCwd == "." {
		return items
	}
	want := strings.TrimRight(m.currentCwd, "/")
	base := cwdBase(want)
	out := make([]sessionItem, 0, len(items))
	for _, it := range items {
		cwd := strings.TrimRight(it.cwd, "/")
		if cwd == want || strings.HasPrefix(cwd, want+"/") {
			out = append(out, it)
			continue
		}
		// worktree / relative labels: match project basename segment
		if base != "" && base != "." && (strings.Contains(cwd, "/"+base+"/") || strings.HasSuffix(cwd, "/"+base) || cwdBase(cwd) == base) {
			out = append(out, it)
		}
	}
	return out
}

func (m *model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// header ~5, footer 1, preview ~7 when visible
	chrome := 6
	previewH := 0
	if !m.showHelp && !m.showStatus {
		previewH = 8
	}
	h := m.height - chrome - previewH
	if h < 5 {
		h = 5
	}
	m.list.SetSize(m.width-2, h)
}

func (m model) selectedID() string {
	if it, ok := m.list.SelectedItem().(sessionItem); ok {
		return it.id
	}
	return ""
}

func (m model) maybePreviewCmd() tea.Cmd {
	id := m.selectedID()
	if id == "" || id == m.previewID {
		return nil
	}
	return fetchPreview(m.cfg, id)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
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
		m.setListItems(msg.items)
		filt := ""
		if m.filterCwd {
			filt = " · filter=cwd"
		}
		m.status = fmt.Sprintf("%d sessions · agent=%s · cwd=%s · wt=%s%s",
			len(m.list.Items()), m.currentAgent, cwdBase(m.currentCwd), onOff(m.formWorktree), filt)
		m.layout()
		return m, m.maybePreviewCmd()

	case agentsMsg:
		if msg.err != nil {
			return m, nil
		}
		m.agents = msg.names
		for i, n := range m.agents {
			if n == m.currentAgent {
				m.agentIdx = i
				break
			}
		}
		if len(m.agents) > 0 {
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
			if m.currentCwd == "" || m.currentCwd == "." {
				m.wsIdx = 0
				m.currentCwd = m.workspaces[0]
			}
		}
		m.status = fmt.Sprintf("cwd → %s", m.currentCwd)
		if m.filterCwd {
			m.setListItems(m.allItems)
		}
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
		m.previewID = ""
		return m, fetchSessions(m.cfg)

	case deletedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.preview = ""
		m.previewID = ""
		m.status = "deleted"
		return m, fetchSessions(m.cfg)

	case resumedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = "resume: " + msg.err.Error()
			return m, fetchSessions(m.cfg)
		}
		if msg.state != "" && msg.state != "running" {
			m.err = fmt.Sprintf("session still %s after resume", msg.state)
			return m, fetchSessions(m.cfg)
		}
		m.attachID = msg.id
		m.quitting = true
		return m, tea.Quit

	case statusMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.showStatus = false
			return m, nil
		}
		m.statusPanel = msg.text
		m.showStatus = true
		m.showHelp = false
		m.err = ""
		m.layout()
		return m, nil

	case previewMsg:
		if msg.id != m.selectedID() {
			// stale
			return m, nil
		}
		m.previewID = msg.id
		if msg.err != nil {
			m.preview = helpStyle.Render("(preview error: " + msg.err.Error() + ")")
		} else {
			m.preview = msg.text
		}
		return m, nil

	case tea.KeyMsg:
		// overlays first
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q", "enter":
				m.showHelp = false
				m.layout()
				return m, nil
			}
			return m, nil
		}
		if m.showStatus {
			switch msg.String() {
			case "s", "esc", "q", "enter":
				m.showStatus = false
				m.layout()
				return m, nil
			}
			return m, nil
		}

		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.attachID = ""
			return m, tea.Quit
		case "?":
			m.showHelp = true
			m.showStatus = false
			m.layout()
			return m, nil
		case "enter", "o":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				if it.state == "running" {
					m.attachID = it.id
					m.quitting = true
					return m, tea.Quit
				}
				// non-running → resume then attach
				m.loading = true
				m.status = "resuming " + shortID(it.id) + "…"
				m.err = ""
				return m, resumeSession(m.cfg, it.id)
			}
		case "n":
			m.loading = true
			wt := onOff(m.formWorktree)
			m.status = fmt.Sprintf("creating %s in %s (wt=%s)…", m.currentAgent, m.currentCwd, wt)
			return m, createSession(m.cfg, m.currentCwd, m.currentAgent, m.formWorktree)
		case "t":
			m.formWorktree = !m.formWorktree
			m.status = fmt.Sprintf("worktree → %s  (press n to start)", onOff(m.formWorktree))
			m.err = ""
			return m, nil
		case "a", "tab":
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
			if len(m.workspaces) == 0 {
				m.err = "no workspaces loaded yet"
				return m, fetchWorkspaces(m.cfg)
			}
			m.wsIdx = (m.wsIdx + 1) % len(m.workspaces)
			m.currentCwd = m.workspaces[m.wsIdx]
			m.status = fmt.Sprintf("cwd → %s  (press n to start)", m.currentCwd)
			m.err = ""
			if m.filterCwd {
				m.setListItems(m.allItems)
			}
			return m, m.maybePreviewCmd()
		case "p":
			m.filterCwd = !m.filterCwd
			m.setListItems(m.allItems)
			if m.filterCwd {
				m.status = fmt.Sprintf("filter cwd → %s  (%d shown)", m.currentCwd, len(m.list.Items()))
			} else {
				m.status = fmt.Sprintf("filter off  (%d sessions)", len(m.list.Items()))
			}
			m.previewID = ""
			return m, m.maybePreviewCmd()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if len(m.agents) == 0 {
				return m, fetchAgents(m.cfg)
			}
			if idx >= 0 && idx < len(m.agents) {
				m.agentIdx = idx
				m.currentAgent = m.agents[idx]
				m.loading = true
				m.status = fmt.Sprintf("creating %s in %s (wt=%s)…", m.currentAgent, m.currentCwd, onOff(m.formWorktree))
				return m, createSession(m.cfg, m.currentCwd, m.currentAgent, m.formWorktree)
			}
		case "r":
			m.loading = true
			m.previewID = ""
			return m, tea.Batch(fetchSessions(m.cfg), fetchAgents(m.cfg), fetchWorkspaces(m.cfg))
		case "x", "d":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				m.loading = true
				m.status = "killing " + shortID(it.id) + "…"
				return m, killSession(m.cfg, it.id)
			}
		case "D":
			if it, ok := m.list.SelectedItem().(sessionItem); ok {
				m.loading = true
				m.status = "deleting " + shortID(it.id) + "…"
				return m, deleteSession(m.cfg, it.id)
			}
		case "s":
			m.loading = true
			m.status = "fetching status…"
			return m, fetchStatus(m.cfg)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	// selection may have changed — refresh preview
	if prev := m.maybePreviewCmd(); prev != nil {
		return m, tea.Batch(cmd, prev)
	}
	return m, cmd
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (m model) View() string {
	if m.quitting && m.attachID == "" {
		return ""
	}
	if m.showHelp {
		return m.viewHelp()
	}
	if m.showStatus {
		return overlayStyle.Render(m.statusPanel)
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

	wtBadge := okStyle.Render("wt:"+onOff(m.formWorktree))
	if !m.formWorktree {
		wtBadge = helpStyle.Render("wt:off")
	}
	cwdLine := "cwd:   " + okStyle.Render(m.currentCwd) + "  " + wtBadge
	if m.filterCwd {
		cwdLine += "  " + warnStyle.Render("filter:cwd")
	}
	if len(m.workspaces) > 1 {
		prev := m.workspaces[(m.wsIdx-1+len(m.workspaces))%len(m.workspaces)]
		next := m.workspaces[(m.wsIdx+1)%len(m.workspaces)]
		cwdLine += "  " + helpStyle.Render(fmt.Sprintf("(w · %d · …%s | %s…)", len(m.workspaces), shortPath(prev), shortPath(next)))
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
	b.WriteString("\n")
	b.WriteString(m.viewPreview())
	b.WriteString("\n" + helpStyle.Render("enter open/resume · n new · t worktree · a agent · w cwd · p filter · x kill · D delete · s status · ? help · q quit"))
	return b.String()
}

func (m model) viewPreview() string {
	title := "preview"
	if id := m.selectedID(); id != "" {
		title = "preview " + shortID(id)
	}
	body := m.preview
	if body == "" {
		body = helpStyle.Render("select a session…")
	}
	// cap height visually
	lines := strings.Split(body, "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	body = strings.Join(lines, "\n")
	block := helpStyle.Render(title) + "\n" + body
	return previewStyle.Width(max(20, m.width-2)).Render(block)
}

func (m model) viewHelp() string {
	keys := []string{
		"enter / o     open session (resume if not running)",
		"n             new session with current agent/cwd/wt",
		"t             toggle worktree for create (default on)",
		"a / tab       cycle agent",
		"w             cycle workspace cwd",
		"p             filter list to current cwd",
		"1-9           quick-pick agent + start",
		"x / d         kill session (keep in list)",
		"D             delete session (remove)",
		"r             refresh sessions / agents / workspaces",
		"s             host status panel",
		"/             filter list (built-in)",
		"?             this help",
		"q             quit (after detach returns to picker)",
		"",
		"detach from PTY returns here — q only exits the TUI",
	}
	body := titleStyle.Render(" keys ") + "\n\n" + strings.Join(keys, "\n")
	body += "\n\n" + helpStyle.Render("press ? or esc to close")
	return overlayStyle.Render(body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run launches the session picker TUI; after PTY detach, re-enters the picker
// until the user quits with q (no attach pending).
func Run(cfg Config) error {
	pstate := prefs{worktree: true}
	for {
		m := newWithPrefs(cfg, pstate)
		prog := tea.NewProgram(m, tea.WithAltScreen())
		final, err := prog.Run()
		if err != nil {
			return err
		}
		fm, ok := final.(model)
		if !ok {
			return nil
		}
		// stash prefs for next picker entry
		pstate = prefs{
			agent:     fm.currentAgent,
			cwd:       fm.currentCwd,
			worktree:  fm.formWorktree,
			filterCwd: fm.filterCwd,
			wsIdx:     fm.wsIdx,
			agentIdx:  fm.agentIdx,
		}
		if fm.attachID == "" {
			return nil
		}
		fmt.Fprintln(os.Stderr, okStyle.Render("→ PTY attach"), fm.attachID)
		if err := clientpty.Attach(cfg.BaseURL, cfg.Token, fm.attachID); err != nil {
			fmt.Fprintln(os.Stderr, errStyle.Render("PTY: "+err.Error()))
		}
		fmt.Fprintln(os.Stderr, helpStyle.Render("… back to session picker"))
		// loop → new picker
	}
}
