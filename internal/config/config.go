package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen            string                 `toml:"listen"`
	JobsDir           string                 `toml:"jobs_dir"`
	WorkspaceRoot     string                 `toml:"workspace_root"`
	MaxConcurrentJobs int                    `toml:"max_concurrent_jobs"`
	DefaultTimeout    string                 `toml:"default_timeout"`
	MaxTimeout        string                 `toml:"max_timeout"`
	Auth              AuthConfig             `toml:"auth"`
	Agents            map[string]AgentConfig `toml:"agents"`
	Allow             AllowConfig            `toml:"allow"`
	Caps              CapsConfig             `toml:"caps"`
	Status            StatusConfig           `toml:"status"`
	Sessions          SessionsConfig         `toml:"sessions"`
	Web               WebConfig              `toml:"web"`
	Memory            MemoryConfig           `toml:"memory"`
	Context           ContextConfig          `toml:"context"`
	Notify            NotifyConfig           `toml:"notify"`
	// DefaultCwd used when client sends empty / "." and "." is not allowlisted.
	DefaultCwd string `toml:"default_cwd"`

	// resolved
	DefaultTimeoutDur time.Duration `toml:"-"`
	MaxTimeoutDur     time.Duration `toml:"-"`
	Token             string        `toml:"-"`
	// Tokens maps label → raw token for multi-token auth (optional).
	// Primary Token from bearer_env is always accepted as label "default".
	TokenMap map[string]string `toml:"-"`
}

// WebConfig controls the embedded browser UI served by agentsd.
type WebConfig struct {
	// Enabled serves the static SPA at / (default true when omitted).
	// Set enabled = false to disable the web UI.
	Enabled *bool `toml:"enabled"`
}

// WebEnabled reports whether the embedded web UI should be mounted.
func (c *Config) WebEnabled() bool {
	if c.Web.Enabled == nil {
		return true
	}
	return *c.Web.Enabled
}

// MemoryConfig is workspace text memory (FTS + optional vectors). Optional; enabled by default.
type MemoryConfig struct {
	// Enabled defaults true when omitted.
	Enabled *bool  `toml:"enabled"`
	Dir     string `toml:"dir"` // default: {jobs_dir}/memory
	// EmbedURL is OpenAI-compatible embeddings base or full .../embeddings URL.
	// Empty = FTS only.
	EmbedURL string `toml:"embed_url"`
	// EmbedModel e.g. text-embedding-3-small or nomic-embed-text
	EmbedModel string `toml:"embed_model"`
	// EmbedAPIKeyEnv is env var name for the embed API key (default AGENTS_EMBED_KEY).
	EmbedAPIKeyEnv string `toml:"embed_api_key_env"`
}

// EmbedAPIKey returns the API key from the configured env var, if any.
func (c *Config) EmbedAPIKey() string {
	env := c.Memory.EmbedAPIKeyEnv
	if env == "" {
		env = "AGENTS_EMBED_KEY"
	}
	return strings.TrimSpace(os.Getenv(env))
}

// MemoryEnabled reports whether memory APIs should be mounted.
func (c *Config) MemoryEnabled() bool {
	if c.Memory.Enabled == nil {
		return true
	}
	return *c.Memory.Enabled
}

// MemoryDir returns the resolved memory data directory.
func (c *Config) MemoryDir() string {
	if c.Memory.Dir != "" {
		return c.Memory.Dir
	}
	return filepath.Join(c.JobsDir, "memory")
}

// ContextConfig controls session orientation (map + pack + seed).
// Defaults favour "ensure + seed" so agents start with better context.
type ContextConfig struct {
	// EnsureOnSession generates/refreshes map+CONTEXT.md on session create (default true).
	EnsureOnSession *bool `toml:"ensure_on_session"`
	// SeedOrientation prepends a short orientation protocol to the TTY seed (default true).
	SeedOrientation *bool `toml:"seed_orientation"`
	// SeedBudget max characters for the TTY seed block (default 1800).
	SeedBudget int `toml:"seed_budget"`
	// PackBudget max characters for .agents/CONTEXT.md (default 12000).
	PackBudget int `toml:"pack_budget"`
	// AutoIndex indexes memory when empty / after map regen (default true).
	AutoIndex *bool `toml:"auto_index"`
	// WriteFiles writes CONTEXT.md + INSTRUCTIONS.md on ensure (default true).
	WriteFiles *bool `toml:"write_files"`
}

func boolDefaultTrue(p *bool) bool {
	if p == nil {
		return true
	}
	return *p
}

// ContextEnsureOnSession reports whether session create should ensure orientation.
func (c *Config) ContextEnsureOnSession() bool {
	return boolDefaultTrue(c.Context.EnsureOnSession)
}

// ContextSeedOrientation reports whether to prepend orientation to TTY seeds.
func (c *Config) ContextSeedOrientation() bool {
	return boolDefaultTrue(c.Context.SeedOrientation)
}

// ContextAutoIndex reports whether ensure should index empty memory.
func (c *Config) ContextAutoIndex() bool {
	return boolDefaultTrue(c.Context.AutoIndex)
}

// ContextWriteFiles reports whether ensure writes CONTEXT.md / INSTRUCTIONS.md.
func (c *Config) ContextWriteFiles() bool {
	return boolDefaultTrue(c.Context.WriteFiles)
}

// ContextSeedBudget returns TTY seed budget.
func (c *Config) ContextSeedBudget() int {
	if c.Context.SeedBudget > 0 {
		return c.Context.SeedBudget
	}
	return 1800
}

// ContextPackBudget returns CONTEXT.md pack budget.
func (c *Config) ContextPackBudget() int {
	if c.Context.PackBudget > 0 {
		return c.Context.PackBudget
	}
	return 12000
}

type AuthConfig struct {
	BearerEnv string `toml:"bearer_env"`
	// ExtraTokens maps label → env var name holding an additional bearer token.
	// Example: extra_tokens = { ops = "AGENTS_TOKEN_OPS", guest = "AGENTS_TOKEN_GUEST" }
	ExtraTokens map[string]string `toml:"extra_tokens"`
	// TrustedHeader: if set (e.g. "Tailscale-User-Login" or "Cf-Access-Authenticated-User-Email"),
	// a non-empty value is accepted as identity alongside (or instead of requiring) bearer when
	// require_bearer is false. When require_bearer is true (default), header is only used for audit actor.
	TrustedHeader string `toml:"trusted_header"`
	// RequireBearer defaults true. Set false only behind a trusted reverse proxy with trusted_header.
	RequireBearer *bool `toml:"require_bearer"`
}

// NotifyConfig webhook notifications.
type NotifyConfig struct {
	WebhookURL string   `toml:"webhook_url"`
	Events     []string `toml:"events"` // empty = all
}

type AgentConfig struct {
	Bin string `toml:"bin"`
	// Args = interactive TTY launch (default for sessions). Empty for claude = plain `claude`.
	Args []string `toml:"args"`
	// PrintArgs = non-interactive / API print mode (jobs with mode=print only). e.g. ["-p"]
	PrintArgs []string `toml:"print_args"`
	// ResumeArgs optional template for process resume after reboot. Use {id} for
	// the native agent conversation id (AgentSessionID). Example for grok:
	//   resume_args = ["--resume", "{id}"]
	// When empty, agentsd uses built-in flags per agent family.
	ResumeArgs []string `toml:"resume_args"`
	// SessionIDArgs optional template to pin a conversation id on create.
	// Use {id} for a newly generated UUID. Example for grok:
	//   session_id_args = ["--session-id", "{id}"]
	SessionIDArgs []string `toml:"session_id_args"`
}

type SessionsConfig struct {
	// SSHHost shown in attach hints, e.g. "agents" or "user@host"
	SSHHost string `toml:"ssh_host"`
	// Display for non-headless browsers (Playwright/Chromium). e.g. ":99" with Xvfb.
	// Empty = leave unset (headless-only environments).
	Display string `toml:"display"`
	// Extra env injected into every agent tmux session (KEY=VALUE).
	Env map[string]string `toml:"env"`
	// PlaywrightBrowsersPath overrides PLAYWRIGHT_BROWSERS_PATH when set.
	PlaywrightBrowsersPath string `toml:"playwright_browsers_path"`
	// PlaywrightServer optional remote Playwright server WS base, e.g. "ws://127.0.0.1:9333"
	PlaywrightServer string `toml:"playwright_server"`
	// Recording archives pane snapshots under jobs_dir/recordings (default false).
	Recording *bool `toml:"recording"`
	// MaxConcurrent caps running TTY sessions (0 = unlimited). Default 0.
	MaxConcurrent int `toml:"max_concurrent"`
	// AutoNote writes a memory note when a session is killed/stopped (default true).
	AutoNote *bool `toml:"auto_note"`
}

// RecordingEnabled reports whether session pane archiving is on.
func (c *Config) RecordingEnabled() bool {
	return c.Sessions.Recording != nil && *c.Sessions.Recording
}

// AutoNoteEnabled defaults true.
func (c *Config) AutoNoteEnabled() bool {
	return boolDefaultTrue(c.Sessions.AutoNote)
}

// RequireBearer defaults true.
func (c *Config) RequireBearer() bool {
	return boolDefaultTrue(c.Auth.RequireBearer)
}

type AllowConfig struct {
	Paths []string `toml:"paths"`
}

type CapsConfig struct {
	Default  []string `toml:"default"`
	Elevated []string `toml:"elevated"`
}

type StatusConfig struct {
	GKEContext  string `toml:"gke_context"`
	OpenDrayURL string `toml:"opendray_url"`
}

func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) normalize() error {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	if c.JobsDir == "" {
		c.JobsDir = "./.jobs"
	}
	if c.WorkspaceRoot == "" {
		c.WorkspaceRoot = "."
	}
	if c.MaxConcurrentJobs <= 0 {
		c.MaxConcurrentJobs = 1
	}
	if c.DefaultTimeout == "" {
		c.DefaultTimeout = "30m"
	}
	if c.MaxTimeout == "" {
		c.MaxTimeout = "2h"
	}
	if c.Auth.BearerEnv == "" {
		c.Auth.BearerEnv = "AGENTSD_TOKEN"
	}
	// Default virtual display for headed Playwright/Chromium when not specified.
	// Operators can set sessions.display = "" to disable.
	if c.Sessions.Display == "" {
		if d := os.Getenv("DISPLAY"); d != "" {
			c.Sessions.Display = d
		}
		// keep empty if no env — setup docs recommend Xvfb :99
	}
	if c.Agents == nil {
		c.Agents = map[string]AgentConfig{}
	}
	if len(c.Caps.Default) == 0 {
		c.Caps.Default = []string{"fs_read", "fs_write", "net_install"}
	}
	if len(c.Caps.Elevated) == 0 {
		c.Caps.Elevated = []string{"git_push", "gh_pr", "kubectl_write", "shell_raw"}
	}

	var err error
	c.DefaultTimeoutDur, err = time.ParseDuration(c.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("default_timeout: %w", err)
	}
	c.MaxTimeoutDur, err = time.ParseDuration(c.MaxTimeout)
	if err != nil {
		return fmt.Errorf("max_timeout: %w", err)
	}

	root, err := filepath.Abs(c.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("workspace_root: %w", err)
	}
	c.WorkspaceRoot = root

	jobs, err := filepath.Abs(c.JobsDir)
	if err != nil {
		return fmt.Errorf("jobs_dir: %w", err)
	}
	c.JobsDir = jobs

	c.Token = strings.TrimSpace(os.Getenv(c.Auth.BearerEnv))
	if c.Token == "" && c.RequireBearer() {
		return fmt.Errorf("auth token empty: set env %s", c.Auth.BearerEnv)
	}
	c.TokenMap = map[string]string{}
	if c.Token != "" {
		c.TokenMap["default"] = c.Token
	}
	for label, envName := range c.Auth.ExtraTokens {
		if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
			c.TokenMap[label] = v
		}
	}
	if len(c.TokenMap) == 0 && c.RequireBearer() {
		return fmt.Errorf("no auth tokens configured")
	}
	return nil
}

// RecordingsDir returns jobs_dir/recordings.
func (c *Config) RecordingsDir() string {
	return filepath.Join(c.JobsDir, "recordings")
}

// IsElevatedCap reports whether cap requires confirm.
func (c *Config) IsElevatedCap(cap string) bool {
	for _, e := range c.Caps.Elevated {
		if e == cap {
			return true
		}
	}
	return false
}

// Agent returns agent config by name (case-insensitive key match on map key).
func (c *Config) Agent(name string) (AgentConfig, bool) {
	if a, ok := c.Agents[name]; ok {
		return a, true
	}
	lower := strings.ToLower(name)
	for k, a := range c.Agents {
		if strings.ToLower(k) == lower {
			return a, true
		}
	}
	return AgentConfig{}, false
}
