package ctlconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config for agentsctl (client).
type Config struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
	// SSHHost used when attaching remotely, e.g. "agents" or "root@192.168.20.6"
	SSHHost string `toml:"ssh_host"`
	// PreferSSH: if true, always ssh attach even when URL is localhost
	// (useful when agentsd is tunneled but tmux is only on the remote host)
	PreferSSH bool `toml:"prefer_ssh"`
}

func DefaultPath() string {
	if x := os.Getenv("AGENTSCTL_CONFIG"); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "agentsctl.toml"
	}
	return filepath.Join(home, ".config", "agentsctl", "config.toml")
}

// Load merges file + env + flags (flags/env win). Empty path skips file.
func Load(path string) (*Config, error) {
	c := &Config{
		URL: "http://127.0.0.1:8787",
	}
	if path == "" {
		path = DefaultPath()
	}
	if b, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(b), c); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if v := os.Getenv("AGENTSCTL_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("AGENTSCTL_TOKEN"); v != "" {
		c.Token = v
	} else if v := os.Getenv("AGENTSD_TOKEN"); v != "" && c.Token == "" {
		c.Token = v
	}
	if v := os.Getenv("AGENTSCTL_SSH_HOST"); v != "" {
		c.SSHHost = v
	}
	if v := os.Getenv("AGENTSCTL_PREFER_SSH"); v == "1" || strings.EqualFold(v, "true") {
		c.PreferSSH = true
	}

	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.SSHHost = strings.TrimSpace(c.SSHHost)
	return c, nil
}

// IsLocalAPI reports whether base URL points at this machine.
func (c *Config) IsLocalAPI() bool {
	u := strings.ToLower(c.URL)
	return strings.Contains(u, "127.0.0.1") ||
		strings.Contains(u, "localhost") ||
		strings.Contains(u, "[::1]")
}

// WriteExample writes a starter config if missing.
func WriteExample(path string) error {
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := `# agentsctl client config (~/.config/agentsctl/config.toml)
# chmod 600 this file

url = "http://127.0.0.1:8787"
token = "replace-with-AGENTSD_TOKEN"

# Optional: only for agentsctl session open --ssh
ssh_host = ""
prefer_ssh = false
`
	return os.WriteFile(path, []byte(body), 0o600)
}
