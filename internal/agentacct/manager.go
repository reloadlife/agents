// Package agentacct integrates cursor-account-switcher (cursor-switch CLI)
// for multi-account Cursor / Claude / Codex / Grok / VS Code profiles.
//
// Global switch: swaps the live auth store (one active account per platform).
// Isolated mode: materializes a profile under a private HOME so parallel
// sessions can run different accounts without clobbering each other.
package agentacct

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Account is a saved profile summary (no secrets).
type Account struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Email   string `json:"email,omitempty"`
	Saved   bool   `json:"saved"`
	Active  bool   `json:"active,omitempty"`
	SavedAt string `json:"saved_at,omitempty"`
}

// PlatformStatus summarizes one tool.
type PlatformStatus struct {
	Platform string    `json:"platform"`
	Current  string    `json:"current,omitempty"` // live email/identifier
	Active   string    `json:"active,omitempty"`  // active profile id
	Accounts []Account `json:"accounts"`
}

// Manager shells out to cursor-switch.
type Manager struct {
	Bin     string // path to cursor-switch
	JobsDir string // for isolated homes
}

// New looks up cursor-switch on PATH (or CURSOR_SWITCH_BIN).
func New(jobsDir string) (*Manager, error) {
	bin := os.Getenv("CURSOR_SWITCH_BIN")
	if bin == "" {
		var err error
		bin, err = exec.LookPath("cursor-switch")
		if err != nil {
			// try common install location
			home, _ := os.UserHomeDir()
			cand := filepath.Join(home, ".local", "bin", "cursor-switch")
			if st, e := os.Stat(cand); e == nil && !st.IsDir() {
				bin = cand
			} else if st, e := os.Stat("/usr/local/bin/cursor-switch"); e == nil && !st.IsDir() {
				bin = "/usr/local/bin/cursor-switch"
			} else {
				return nil, fmt.Errorf("cursor-switch not found — install from https://github.com/reloadlife/cursor-account-switcher")
			}
		}
	}
	return &Manager{Bin: bin, JobsDir: jobsDir}, nil
}

func (m *Manager) run(timeout time.Duration, args ...string) (string, error) {
	cmd := exec.Command(m.Bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		out := buf.String()
		if err != nil {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
		}
		return out, nil
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("cursor-switch timed out")
	}
}

// Platforms returns known platform ids.
func (m *Manager) Platforms() []string {
	return []string{"cursor", "claude", "codex", "grok", "vscode"}
}

// MapAgentToPlatform maps agentsd agent name → switcher platform.
func MapAgentToPlatform(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	switch {
	case strings.Contains(a, "cursor"):
		return "cursor"
	case strings.Contains(a, "claude"):
		return "claude"
	case strings.Contains(a, "codex") || a == "gpt":
		return "codex"
	case strings.Contains(a, "grok"):
		return "grok"
	case strings.Contains(a, "copilot") || a == "vscode":
		return "vscode"
	default:
		return ""
	}
}

// Status returns accounts for a platform by parsing cursor-switch status.
func (m *Manager) Status(platform string) (*PlatformStatus, error) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = "cursor"
	}
	out, err := m.run(20*time.Second, "--platform", platform, "status")
	// parse even on non-zero if partial
	st := parseStatus(platform, out)
	if err != nil && len(st.Accounts) == 0 {
		return nil, err
	}
	return st, nil
}

// ListAll statuses for every platform (best-effort).
func (m *Manager) ListAll() ([]PlatformStatus, error) {
	var out []PlatformStatus
	for _, p := range m.Platforms() {
		st, err := m.Status(p)
		if err != nil {
			out = append(out, PlatformStatus{Platform: p, Accounts: []Account{}})
			continue
		}
		out = append(out, *st)
	}
	return out, nil
}

// Save current live login as profile id.
func (m *Manager) Save(platform, id, label string) error {
	args := []string{"--platform", platform, "save", id}
	if label != "" {
		args = append(args, "--label", label)
	}
	_, err := m.run(60*time.Second, args...)
	return err
}

// Switch globally restores a profile into the live auth store.
func (m *Manager) Switch(platform, id string) error {
	_, err := m.run(90*time.Second, "--platform", platform, "switch", id)
	return err
}

// AddAccount registers a new account id/label without saving auth yet.
func (m *Manager) AddAccount(platform, id, label string) error {
	args := []string{"--platform", platform, "account", "add", id}
	if label != "" {
		args = append(args, "--label", label)
	}
	_, err := m.run(15*time.Second, args...)
	return err
}

// Materialize expands a saved profile into an isolated HOME directory.
func (m *Manager) Materialize(platform, id, destHome string) (string, error) {
	if destHome == "" {
		return "", fmt.Errorf("dest home required")
	}
	out, err := m.run(60*time.Second, "--platform", platform, "materialize", id, "--dest", destHome)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(out)
	if path == "" {
		path = destHome
	}
	return path, nil
}

// IsolatedHome returns the default sandbox path for a platform/account.
func (m *Manager) IsolatedHome(platform, id string) string {
	return filepath.Join(m.JobsDir, "accounts", platform, id, "home")
}

// EnsureIsolated materializes profile into jobs_dir sandbox and returns HOME path.
func (m *Manager) EnsureIsolated(platform, id string) (string, error) {
	dest := m.IsolatedHome(platform, id)
	return m.Materialize(platform, id, dest)
}

// SessionEnv returns env vars so an agent process uses an isolated account.
// For file-based CLIs (grok, codex, claude) HOME isolation is enough when
// the profile was materialized under that HOME.
func SessionEnv(base []string, platform, isolatedHome string) []string {
	if isolatedHome == "" {
		return base
	}
	env := append([]string{}, base...)
	env = setEnv(env, "HOME", isolatedHome)
	// Tool-specific overrides when CLIs honor them
	switch platform {
	case "grok":
		env = setEnv(env, "GROK_HOME", filepath.Join(isolatedHome, ".grok"))
		env = setEnv(env, "GROK_CONFIG_DIR", filepath.Join(isolatedHome, ".grok"))
	case "codex":
		env = setEnv(env, "CODEX_HOME", filepath.Join(isolatedHome, ".codex"))
	case "claude":
		env = setEnv(env, "CLAUDE_CONFIG_DIR", filepath.Join(isolatedHome, ".claude"))
		// also place .claude.json at home root via materialize
	case "cursor":
		env = setEnv(env, "XDG_CONFIG_HOME", filepath.Join(isolatedHome, ".config"))
	}
	// ensure PATH still works — prepend common bins from real home
	if realHome, err := os.UserHomeDir(); err == nil {
		extra := []string{
			filepath.Join(realHome, ".local", "bin"),
			filepath.Join(realHome, ".grok", "bin"),
			"/usr/local/bin",
		}
		// keep existing PATH but with extras
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + strings.Join(extra, ":") + ":" + e[5:]
				break
			}
		}
	}
	return env
}

func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

// parseStatus is a loose text parser for cursor-switch status output.
func parseStatus(platform, raw string) *PlatformStatus {
	st := &PlatformStatus{Platform: platform, Accounts: []Account{}}
	// Also try reading config JSON directly for reliability.
	// Linux: ~/.config/cursor-account-switcher ; macOS: ~/.cursor-account-switcher
	if home, err := os.UserHomeDir(); err == nil {
		roots := []string{
			filepath.Join(home, ".config", "cursor-account-switcher"),
			filepath.Join(home, ".cursor-account-switcher"),
		}
		for _, root := range roots {
			cfgPath := filepath.Join(root, platform, "config.json")
			b, err := os.ReadFile(cfgPath)
			if err != nil {
				continue
			}
			var cfg struct {
				ActiveAccount *string `json:"activeAccount"`
				Accounts      []struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"accounts"`
			}
			if json.Unmarshal(b, &cfg) != nil {
				continue
			}
			active := ""
			if cfg.ActiveAccount != nil {
				active = *cfg.ActiveAccount
				st.Active = active
			}
			for _, a := range cfg.Accounts {
				acc := Account{ID: a.ID, Label: a.Label, Active: a.ID == active}
				prof := filepath.Join(root, platform, "profiles", a.ID+".json")
				if stt, err := os.Stat(prof); err == nil && !stt.IsDir() {
					acc.Saved = true
					var p struct {
						Email   *string `json:"email"`
						SavedAt string  `json:"savedAt"`
					}
					if pb, err := os.ReadFile(prof); err == nil && json.Unmarshal(pb, &p) == nil {
						if p.Email != nil {
							acc.Email = *p.Email
						}
						acc.SavedAt = p.SavedAt
					}
				}
				st.Accounts = append(st.Accounts, acc)
			}
			break
		}
	}
	// Current line from status text
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Current account:") {
			st.Current = strings.TrimSpace(strings.TrimPrefix(line, "Current account:"))
		}
	}
	return st
}
