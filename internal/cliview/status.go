package cliview

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	stLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(12)
	stValue  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	stBad    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	stWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	stMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	stHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
)

// RenderStatus pretty-prints /v1/status JSON for humans.
func RenderStatus(w io.Writer, raw []byte, apiURL string) error {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}

	host := str(m["host"])
	goos := str(m["goos"])
	arch := str(m["goarch"])
	cpus := intVal(m["num_cpu"])
	uptime := formatUptime(floatVal(m["uptime_sec"]))
	when := str(m["time"])
	if t, err := time.Parse(time.RFC3339Nano, when); err == nil {
		when = t.Local().Format("2006-01-02 15:04:05 MST")
	}

	load := formatLoad(m["load"])
	mem := m["mem"]
	alloc, sys := memPair(mem)

	diskLine := formatDisk(m["disk"])
	docker := badge(str(m["docker"]))
	opendray := badge(str(m["opendray"]))
	gke := formatGKE(m["gke"])
	jobs := formatJobs(m["jobs"])
	tty := intVal(m["tty_sessions"])

	title := stTitle.Render(" agents ") + stMuted.Render(host)
	if apiURL != "" {
		title += stMuted.Render("  ·  "+apiURL)
	}

	agentsLine := formatAgents(m["agents"])

	body := strings.Join([]string{
		row("host", fmt.Sprintf("%s  %s/%s  %d cpu", host, goos, arch, cpus)),
		row("uptime", uptime),
		row("time", when),
		row("load", load),
		row("memory", formatMem(m["mem"], alloc, sys)),
		row("disk", diskLine),
		row("docker", docker),
		row("opendray", opendray),
		row("gke", gke),
		row("jobs", jobs),
		row("tty", fmt.Sprintf("%d interactive session(s)", tty)),
		row("agents", agentsLine),
	}, "\n")

	fmt.Fprintln(w, stBox.Render(title+"\n\n"+body))
	fmt.Fprintln(w, stMuted.Render("  tui: agentsctl tui   ·   start: agentsctl session start -a grok --open   ·   --json"))
	return nil
}

func formatAgents(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return stMuted.Render("—")
	}
	var parts []string
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		name := str(m["name"])
		if name == "mock" || name == "cursor-agent" {
			continue // hide noise / alias
		}
		if b, _ := m["available"].(bool); b {
			parts = append(parts, stOK.Render(name))
		} else {
			parts = append(parts, stMuted.Render(name+"✗"))
		}
	}
	if len(parts) == 0 {
		return stMuted.Render("none")
	}
	return strings.Join(parts, "  ")
}

// RenderSessionList pretty table for sessions.
func RenderSessionList(w io.Writer, raw []byte) error {
	var out struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if len(out.Sessions) == 0 {
		fmt.Fprintln(w, stMuted.Render("no sessions")+"  "+stMuted.Render("→ agentsctl tui  or  session start -r REPO -a claude --open"))
		return nil
	}

	fmt.Fprintln(w, stHeader.Render(" SESSIONS "))
	fmt.Fprintf(w, "%s\n", stMuted.Render(fmt.Sprintf("  %-28s  %-10s  %-8s  %-20s  %s", "ID", "STATE", "AGENT", "TMUX", "CWD")))
	for _, s := range out.Sessions {
		state := str(s["state"])
		st := stValue.Render(state)
		switch state {
		case "running":
			st = stOK.Render(state)
		case "killed", "exited":
			st = stMuted.Render(state)
		}
		fmt.Fprintf(w, "  %-28s  %-18s  %-8s  %-20s  %s\n",
			str(s["id"]), st, str(s["agent"]), str(s["tmux"]), str(s["cwd"]))
	}
	fmt.Fprintln(w, stMuted.Render("\n  open: agentsctl session open <id>   ·   tui: agentsctl tui"))
	return nil
}

func row(label, value string) string {
	return stLabel.Render(label) + "  " + stValue.Render(value)
}

func badge(s string) string {
	switch strings.ToLower(s) {
	case "active", "up", "ok", "running":
		return stOK.Render("● " + s)
	case "down", "error", "failed", "":
		if s == "" {
			s = "unknown"
		}
		return stBad.Render("● " + s)
	default:
		return stWarn.Render("● " + s)
	}
}

func formatMem(v any, alloc, sys uint64) string {
	base := fmt.Sprintf("daemon %d MB", alloc)
	m, ok := v.(map[string]any)
	if !ok {
		return base
	}
	total := uint64(floatVal(m["host_total_mb"]))
	avail := uint64(floatVal(m["host_available_mb"]))
	if total > 0 {
		usedPct := 100 - int(avail*100/total)
		color := stOK
		if usedPct >= 90 {
			color = stBad
		} else if usedPct >= 75 {
			color = stWarn
		}
		return fmt.Sprintf("host %s / %d MB free  %s  · daemon %d MB",
			formatMB(avail), total, color.Render(fmt.Sprintf("%d%% used", usedPct)), alloc)
	}
	return fmt.Sprintf("%s · sys %d MB", base, sys)
}

func formatMB(n uint64) string {
	if n >= 1024 {
		return fmt.Sprintf("%.1f GiB", float64(n)/1024)
	}
	return fmt.Sprintf("%d MB", n)
}

func formatLoad(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return stMuted.Render("n/a")
	}
	parts := make([]string, 0, len(arr))
	for _, x := range arr {
		parts = append(parts, fmt.Sprintf("%.2f", floatVal(x)))
	}
	return strings.Join(parts, "  ")
}

func formatDisk(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return stMuted.Render("n/a")
	}
	raw := str(m["raw"])
	// parse last non-header line of df -h
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[len(lines)-1])
		// Filesystem Size Used Avail Use% Mounted
		if len(fields) >= 5 {
			use := fields[4]
			color := stOK
			if p, err := strconv.Atoi(strings.TrimSuffix(use, "%")); err == nil {
				if p >= 90 {
					color = stBad
				} else if p >= 75 {
					color = stWarn
				}
			}
			return fmt.Sprintf("%s  used %s  avail %s  %s  %s",
				str(m["path"]), fields[2], fields[3], color.Render(use), fields[len(fields)-1])
		}
	}
	if raw != "" {
		return strings.ReplaceAll(raw, "\n", " · ")
	}
	return str(m["path"])
}

func formatGKE(v any) string {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return stMuted.Render("not configured")
	}
	if err := str(m["error"]); err != "" {
		return stWarn.Render("unavailable") + stMuted.Render("  ("+truncate(err, 48)+")")
	}
	ready := intVal(m["nodes_ready"])
	total := intVal(m["nodes_total"])
	if total == 0 {
		return stMuted.Render("no nodes")
	}
	if ready == total {
		return stOK.Render(fmt.Sprintf("%d/%d Ready", ready, total))
	}
	return stWarn.Render(fmt.Sprintf("%d/%d Ready", ready, total))
}

func formatJobs(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("print jobs  running %d · queued %d", intVal(m["running"]), intVal(m["queued"]))
}

func memPair(v any) (alloc, sys uint64) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, 0
	}
	return uint64(floatVal(m["alloc_mb"])), uint64(floatVal(m["sys_mb"]))
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
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func floatVal(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func intVal(v any) int {
	return int(floatVal(v))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
