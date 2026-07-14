// Package ghauth manages GitHub CLI (`gh`) accounts on the agents host.
// Authentication tokens are never returned over the API — only account metadata.
package ghauth

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Account is a logged-in gh identity (no token fields).
type Account struct {
	Host        string   `json:"host"`
	User        string   `json:"user"`
	Active      bool     `json:"active"`
	GitProtocol string   `json:"git_protocol,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	ConfigFile  string   `json:"config_file,omitempty"`
}

// Status is the full auth picture for the server.
type Status struct {
	OK       bool      `json:"ok"`
	Binary   string    `json:"binary,omitempty"`
	Accounts []Account `json:"accounts"`
	Active   string    `json:"active,omitempty"` // user@host of active account if any
	Raw      string    `json:"raw,omitempty"`    // truncated status text (no tokens)
	Error    string    `json:"error,omitempty"`
}

// LoginRequest adds an account via `gh auth login --with-token`.
type LoginRequest struct {
	// Token: PAT or fine-grained token (write-only; never stored in responses)
	Token string `json:"token"`
	// Host: default github.com
	Host string `json:"host,omitempty"`
	// GitProtocol: https (default) or ssh
	GitProtocol string `json:"git_protocol,omitempty"`
	// InsecureStorage: force hosts.yml plain text (often needed headless)
	InsecureStorage bool `json:"insecure_storage,omitempty"`
	// Scopes: optional extra scopes (comma-separated list items)
	Scopes []string `json:"scopes,omitempty"`
}

// SwitchRequest selects the active account.
type SwitchRequest struct {
	Host string `json:"host,omitempty"`
	User string `json:"user"`
}

// LogoutRequest removes a local gh account config (does not revoke the token).
type LogoutRequest struct {
	Host string `json:"host,omitempty"`
	User string `json:"user,omitempty"`
}

// Manager shells out to `gh`.
type Manager struct {
	bin string
	env []string
}

// New creates a manager; bin defaults to "gh" on PATH.
func New() (*Manager, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("gh CLI not found on PATH")
	}
	return &Manager{bin: bin, env: os.Environ()}, nil
}

func (m *Manager) run(timeout time.Duration, args ...string) (string, error) {
	cmd := exec.Command(m.bin, args...)
	cmd.Env = m.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		out := stdout.String() + stderr.String()
		if err != nil {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
		}
		return out, nil
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("gh timed out after %s", timeout)
	}
}

// Status returns parsed gh auth status (tokens redacted).
func (m *Manager) Status() (*Status, error) {
	out, err := m.run(15*time.Second, "auth", "status")
	st := &Status{
		Binary:   m.bin,
		Accounts: []Account{},
		Raw:      redactTokens(out),
	}
	if err != nil {
		// gh exits non-zero when not logged in
		st.OK = false
		st.Error = strings.TrimSpace(redactTokens(err.Error()))
		// still try parse partial output
		st.Accounts = parseStatus(out)
		if len(st.Accounts) > 0 {
			st.OK = true
			st.Error = ""
		}
		fillActive(st)
		return st, nil
	}
	st.OK = true
	st.Accounts = parseStatus(out)
	fillActive(st)
	return st, nil
}

func fillActive(st *Status) {
	for _, a := range st.Accounts {
		if a.Active {
			st.Active = a.User + "@" + a.Host
			return
		}
	}
}

// Login stores a token via gh auth login --with-token.
func (m *Manager) Login(req LoginRequest) (*Status, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "github.com"
	}
	proto := strings.ToLower(strings.TrimSpace(req.GitProtocol))
	if proto == "" {
		proto = "https"
	}
	if proto != "https" && proto != "ssh" {
		return nil, fmt.Errorf("git_protocol must be https or ssh")
	}

	args := []string{
		"auth", "login",
		"--hostname", host,
		"--git-protocol", proto,
		"--with-token",
	}
	if req.InsecureStorage {
		args = append(args, "--insecure-storage")
	}
	for _, s := range req.Scopes {
		s = strings.TrimSpace(s)
		if s != "" {
			args = append(args, "--scopes", s)
		}
	}

	cmd := exec.Command(m.bin, args...)
	cmd.Env = m.env
	cmd.Stdin = strings.NewReader(token + "\n")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh auth login: %w (%s)", err, redactTokens(strings.TrimSpace(buf.String())))
	}
	return m.Status()
}

// Switch sets the active account.
func (m *Manager) Switch(req SwitchRequest) (*Status, error) {
	user := strings.TrimSpace(req.User)
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "github.com"
	}
	args := []string{"auth", "switch", "--hostname", host, "--user", user}
	if out, err := m.run(15*time.Second, args...); err != nil {
		return nil, fmt.Errorf("gh auth switch: %w (%s)", err, redactTokens(out))
	}
	return m.Status()
}

// Logout removes a local account configuration.
func (m *Manager) Logout(req LogoutRequest) (*Status, error) {
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "github.com"
	}
	args := []string{"auth", "logout", "--hostname", host}
	if u := strings.TrimSpace(req.User); u != "" {
		args = append(args, "--user", u)
	}
	if out, err := m.run(15*time.Second, args...); err != nil {
		return nil, fmt.Errorf("gh auth logout: %w (%s)", err, redactTokens(out))
	}
	return m.Status()
}

// SetupGit configures git to use gh as credential helper.
func (m *Manager) SetupGit() error {
	if out, err := m.run(30*time.Second, "auth", "setup-git"); err != nil {
		return fmt.Errorf("gh auth setup-git: %w (%s)", err, redactTokens(out))
	}
	return nil
}

var (
	reHostLine   = regexp.MustCompile(`^([a-zA-Z0-9._-]+)$`)
	reAccount    = regexp.MustCompile(`Logged in to (\S+) account (\S+)`)
	reActive     = regexp.MustCompile(`Active account:\s*(true|false)`)
	reProtocol   = regexp.MustCompile(`Git operations protocol:\s*(\S+)`)
	reScopes     = regexp.MustCompile(`Token scopes:\s*(.+)`)
	reConfigFile = regexp.MustCompile(`\(([^)]+)\)\s*$`)
	reTokenLine  = regexp.MustCompile(`(?i)(token|oauth)[:\s]+\S+`)
	reGho        = regexp.MustCompile(`\b(gho_|ghp_|ghu_|ghs_|github_pat_)[A-Za-z0-9_]+\b`)
)

func parseStatus(raw string) []Account {
	var accounts []Account
	var host string
	var cur *Account

	flush := func() {
		if cur != nil && cur.User != "" {
			if cur.Host == "" {
				cur.Host = host
			}
			accounts = append(accounts, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(raw, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		// top-level host
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && reHostLine.MatchString(trim) {
			flush()
			host = trim
			continue
		}
		if m := reAccount.FindStringSubmatch(trim); len(m) == 3 {
			flush()
			cur = &Account{Host: m[1], User: m[2]}
			if cf := reConfigFile.FindStringSubmatch(trim); len(cf) == 2 {
				cur.ConfigFile = cf[1]
			}
			continue
		}
		if cur == nil {
			continue
		}
		if m := reActive.FindStringSubmatch(trim); len(m) == 2 {
			cur.Active = m[1] == "true"
		}
		if m := reProtocol.FindStringSubmatch(trim); len(m) == 2 {
			cur.GitProtocol = m[1]
		}
		if m := reScopes.FindStringSubmatch(trim); len(m) == 2 {
			cur.Scopes = parseScopes(m[1])
		}
	}
	flush()
	return accounts
}

func parseScopes(s string) []string {
	s = strings.TrimSpace(s)
	// 'gist', 'read:org', 'repo'
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'\"")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func redactTokens(s string) string {
	s = reGho.ReplaceAllString(s, "***")
	// redact "Token: xxx" lines content
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if reTokenLine.MatchString(line) && (strings.Contains(strings.ToLower(line), "token:") || strings.Contains(strings.ToLower(line), "token ")) {
			// keep structure, hide value
			if idx := strings.Index(strings.ToLower(line), "token"); idx >= 0 {
				// find colon after token
				rest := line[idx:]
				if c := strings.Index(rest, ":"); c >= 0 {
					lines[i] = line[:idx+c+1] + " ***"
					continue
				}
			}
			lines[i] = reGho.ReplaceAllString(line, "***")
		}
	}
	return strings.Join(lines, "\n")
}
