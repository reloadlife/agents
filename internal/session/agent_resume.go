package session

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/reloadlife/agents/internal/config"
)

// IsShellAgent reports whether this is a plain shell terminal (not an AI agent).
func IsShellAgent(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "shell" || n == "term" || n == "terminal" || n == "bash" || n == "zsh" || n == "sh"
}

// defaultShell picks an interactive shell binary.
func defaultShell() string {
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		if st, err := os.Stat(s); err == nil && !st.IsDir() {
			return s
		}
	}
	for _, c := range []string{"/bin/zsh", "/usr/bin/zsh", "/bin/bash", "/usr/bin/bash", "/bin/sh"} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "/bin/sh"
}

// Agent family helpers — map config agent names to native CLI resume behaviour.
func agentFamily(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case IsShellAgent(n):
		return "shell"
	case strings.Contains(n, "grok"):
		return "grok"
	case strings.Contains(n, "claude"):
		return "claude"
	case strings.Contains(n, "codex"):
		return "codex"
	case strings.Contains(n, "opencode"):
		return "opencode"
	case strings.Contains(n, "cursor"):
		return "cursor"
	default:
		return n
	}
}

// canPinSessionID reports whether the CLI accepts a fixed conversation id on create.
func canPinSessionID(agent string) bool {
	switch agentFamily(agent) {
	case "grok":
		// grok --session-id <UUID>
		return true
	default:
		return false
	}
}

// newNativeSessionID returns a UUID for CLIs that require one on create.
func newNativeSessionID() string {
	return uuid.NewString()
}

// expandPlaceholders replaces {id} in arg templates.
func expandPlaceholders(args []string, id string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{id}", id)
	}
	return out
}

// ttyLaunchArgs builds interactive CLI args for create or resume.
//
//   - create + pin: e.g. grok --session-id <uuid>
//   - resume + id:  e.g. grok --resume <uuid>, claude --resume <id>
//   - resume, no id: best-effort continue/last for the cwd
func ttyLaunchArgs(agentName string, acfg config.AgentConfig, nativeID string, resume bool) []string {
	// Config overrides win when set.
	if resume && nativeID != "" && len(acfg.ResumeArgs) > 0 {
		return expandPlaceholders(append([]string{}, acfg.ResumeArgs...), nativeID)
	}
	if !resume && nativeID != "" && len(acfg.SessionIDArgs) > 0 {
		return expandPlaceholders(append([]string{}, acfg.SessionIDArgs...), nativeID)
	}

	base := append([]string{}, acfg.Args...)
	fam := agentFamily(agentName)

	if resume {
		if nativeID != "" {
			switch fam {
			case "grok":
				return append(base, "--resume", nativeID)
			case "claude":
				return append(base, "--resume", nativeID)
			case "codex":
				// subcommand form: codex resume <id>
				return append(base, "resume", nativeID)
			case "opencode":
				return append(base, "--session", nativeID)
			default:
				// unknown: bare relaunch (no chat restore)
				return base
			}
		}
		// No stored id — continue most recent for this cwd when the CLI supports it.
		switch fam {
		case "grok":
			return append(base, "--continue")
		case "claude":
			return append(base, "--continue")
		case "codex":
			return append(base, "resume", "--last")
		case "opencode":
			return append(base, "--continue")
		default:
			return base
		}
	}

	// Fresh create — pin id when the CLI supports it.
	if nativeID != "" {
		switch fam {
		case "grok":
			return append(base, "--session-id", nativeID)
		}
	}
	return base
}

// homeForSession returns the HOME that holds agent state for this session
// (isolated account home when set, else process HOME / user home).
func homeForSession(s *Session) string {
	if s != nil && s.AccountHome != "" {
		return s.AccountHome
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// discoverNativeSessionID tries to find an existing agent conversation id for
// legacy sessions that predate AgentSessionID persistence (post-reboot recovery).
func discoverNativeSessionID(s *Session) string {
	if s == nil {
		return ""
	}
	home := homeForSession(s)
	cwd := s.CwdAbs
	if cwd == "" {
		cwd = s.Cwd
	}
	if home == "" || cwd == "" {
		return ""
	}
	switch agentFamily(s.Agent) {
	case "grok":
		return discoverGrokSessionID(home, cwd, s.CreatedAt)
	default:
		return ""
	}
}

// discoverGrokSessionID picks the best session under ~/.grok/sessions/<enc(cwd)>/.
// Prefer the most recently modified directory (post-reboot: last chat for that cwd).
// createdAt, when set, slightly prefers dirs modified after session create (weak signal).
func discoverGrokSessionID(home, cwdAbs string, createdAt time.Time) string {
	dir := filepath.Join(home, ".grok", "sessions", url.QueryEscape(cwdAbs))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best, bestRecent string
	var bestMod, bestRecentMod time.Time
	floor := time.Time{}
	if !createdAt.IsZero() {
		floor = createdAt.Add(-5 * time.Minute)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Grok session dirs look like UUIDs (with hyphens).
		if strings.Count(name, "-") < 2 || len(name) < 20 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mod := info.ModTime()
		if best == "" || mod.After(bestMod) {
			best = name
			bestMod = mod
		}
		if !floor.IsZero() && !mod.Before(floor) {
			if bestRecent == "" || mod.After(bestRecentMod) {
				bestRecent = name
				bestRecentMod = mod
			}
		}
	}
	if bestRecent != "" {
		return bestRecent
	}
	return best
}
