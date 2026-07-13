package agentsinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/reloadlife/agents/internal/config"
)

// Info describes one configured agent and whether its binary is available.
type Info struct {
	Name      string   `json:"name"`
	Bin       string   `json:"bin"`
	Resolved  string   `json:"resolved,omitempty"`
	Available bool     `json:"available"`
	Args      []string `json:"args,omitempty"`
	TTY       bool     `json:"tty"` // true = interactive session supported
	Print     bool     `json:"print"`
	Note      string   `json:"note,omitempty"`
}

// Preferred display order for the catalog.
var order = []string{"claude", "grok", "codex", "opencode", "cursor", "cursor-agent", "mock"}

// List returns configured agents, resolved against PATH.
func List(cfg *config.Config) []Info {
	seen := map[string]bool{}
	var out []Info

	// stable order first
	for _, name := range order {
		if ac, ok := cfg.Agents[name]; ok {
			out = append(out, infoFor(name, ac))
			seen[name] = true
		}
	}
	// any extras alphabetically
	var extra []string
	for name := range cfg.Agents {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		out = append(out, infoFor(name, cfg.Agents[name]))
	}
	return out
}

// AvailableTTY returns names suitable for interactive sessions (bin found, not mock-only noise).
func AvailableTTY(cfg *config.Config) []string {
	var names []string
	for _, a := range List(cfg) {
		if a.Available && a.TTY && a.Name != "mock" {
			// hide cursor-agent duplicate if cursor exists
			if a.Name == "cursor-agent" {
				if _, ok := cfg.Agents["cursor"]; ok {
					if p, _ := exec.LookPath(cfg.Agents["cursor"].Bin); p != "" {
						continue
					}
				}
			}
			names = append(names, a.Name)
		}
	}
	return names
}

func infoFor(name string, ac config.AgentConfig) Info {
	inf := Info{
		Name:  name,
		Bin:   ac.Bin,
		Args:  ac.Args,
		TTY:   true,
		Print: len(ac.PrintArgs) > 0,
	}
	if ac.Bin == "" {
		inf.Available = false
		inf.Note = "empty bin"
		return inf
	}
	// absolute path
	if filepath.IsAbs(ac.Bin) {
		if st, err := os.Stat(ac.Bin); err == nil && !st.IsDir() {
			inf.Resolved = ac.Bin
			inf.Available = true
		} else {
			inf.Note = "bin not found"
		}
		return inf
	}
	if p, err := exec.LookPath(ac.Bin); err == nil {
		inf.Resolved = p
		inf.Available = true
	} else {
		home, _ := os.UserHomeDir()
		if h := os.Getenv("HOME"); h != "" {
			home = h
		}
		for _, dir := range []string{
			filepath.Join(home, ".local", "bin"),
			"/usr/local/bin",
			filepath.Join(home, ".grok", "bin"),
			filepath.Join(home, ".opencode", "bin"),
		} {
			if dir == "" {
				continue
			}
			cand := filepath.Join(dir, ac.Bin)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				inf.Resolved = cand
				inf.Available = true
				return inf
			}
		}
		inf.Note = "not on PATH"
	}
	switch strings.ToLower(name) {
	case "claude":
		inf.Note = "Claude Code interactive (subscription)"
	case "grok":
		inf.Note = "Grok Build TUI"
	case "codex":
		inf.Note = "OpenAI Codex interactive"
	case "opencode":
		inf.Note = "OpenCode TUI"
	case "cursor", "cursor-agent":
		inf.Note = "Cursor Agent interactive"
	case "mock":
		inf.Note = "test echo agent"
	}
	return inf
}
