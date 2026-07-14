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
	// DefaultCwd used when client sends empty / "." and "." is not allowlisted.
	DefaultCwd string `toml:"default_cwd"`

	// resolved
	DefaultTimeoutDur time.Duration `toml:"-"`
	MaxTimeoutDur     time.Duration `toml:"-"`
	Token             string        `toml:"-"`
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

type AuthConfig struct {
	BearerEnv string `toml:"bearer_env"`
}

type AgentConfig struct {
	Bin string `toml:"bin"`
	// Args = interactive TTY launch (default for sessions). Empty for claude = plain `claude`.
	Args []string `toml:"args"`
	// PrintArgs = non-interactive / API print mode (jobs with mode=print only). e.g. ["-p"]
	PrintArgs []string `toml:"print_args"`
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
	if c.Token == "" {
		return fmt.Errorf("auth token empty: set env %s", c.Auth.BearerEnv)
	}
	return nil
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
