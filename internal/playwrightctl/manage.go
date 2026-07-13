package playwrightctl

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/config"
)

// Status is the Playwright/display stack health.
type Status struct {
	Display       string `json:"display"`
	DisplayOK     string `json:"display_ok"` // active|down|unset
	Xvfb          string `json:"xvfb"`       // active|inactive|missing
	Container     string `json:"container"`  // running|exited|missing|unknown
	ContainerName string `json:"container_name"`
	Server        string `json:"server,omitempty"` // configured PLAYWRIGHT_SERVER
	ServerOK      string `json:"server_ok"`       // open|closed|unset
	BrowsersPath  string `json:"browsers_path,omitempty"`
	BrowsersOK    bool   `json:"browsers_ok"`
	ComposeFile   string `json:"compose_file,omitempty"`
	Message       string `json:"message,omitempty"`
}

// Manager runs host-side Playwright/Xvfb/docker operations.
type Manager struct {
	cfg         *config.Config
	composeFile string
	composeDir  string
	container   string
}

func New(cfg *config.Config) *Manager {
	// Prefer compose next to binary's workspace or common install paths
	candidates := []string{
		filepath.Join(cfg.WorkspaceRoot, "local-agents", "deploy", "docker-compose.playwright.yml"),
		filepath.Join(cfg.WorkspaceRoot, "agents", "deploy", "docker-compose.playwright.yml"),
		"/root/workspace/local-agents/deploy/docker-compose.playwright.yml",
		"/root/workspace/agents/deploy/docker-compose.playwright.yml",
		"deploy/docker-compose.playwright.yml",
	}
	// also walk from cwd
	if wd, err := os.Getwd(); err == nil {
		candidates = append([]string{
			filepath.Join(wd, "deploy", "docker-compose.playwright.yml"),
		}, candidates...)
	}
	var compose string
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			compose = c
			break
		}
	}
	return &Manager{
		cfg:         cfg,
		composeFile: compose,
		composeDir:  filepath.Dir(compose),
		container:   "agents-playwright",
	}
}

func (m *Manager) Status() Status {
	st := Status{
		ContainerName: m.container,
		ComposeFile:   m.composeFile,
		Server:        m.cfg.Sessions.PlaywrightServer,
	}
	st.Display = m.cfg.Sessions.Display
	if st.Display == "" {
		st.Display = os.Getenv("DISPLAY")
	}
	st.DisplayOK = probeDisplay(st.Display)
	st.Xvfb = unitState("xvfb")
	st.Container = m.containerState()
	st.ServerOK = probePort(st.Server)
	st.BrowsersPath = m.browsersPath()
	if st.BrowsersPath != "" {
		if entries, err := os.ReadDir(st.BrowsersPath); err == nil && len(entries) > 0 {
			st.BrowsersOK = true
		}
	}
	st.Message = summarize(st)
	return st
}

func (m *Manager) Start(ctx context.Context) (Status, error) {
	// 1) Xvfb
	if err := run(ctx, "systemctl", "start", "xvfb"); err != nil {
		// try enable+start
		_ = run(ctx, "systemctl", "enable", "xvfb")
		if err2 := run(ctx, "systemctl", "start", "xvfb"); err2 != nil {
			return m.Status(), fmt.Errorf("start xvfb: %w", err2)
		}
	}
	// 2) docker compose
	if m.composeFile == "" {
		return m.Status(), fmt.Errorf("compose file not found (deploy/docker-compose.playwright.yml)")
	}
	if err := run(ctx, "docker", "compose", "-f", m.composeFile, "up", "-d"); err != nil {
		return m.Status(), fmt.Errorf("docker compose up: %w", err)
	}
	// wait briefly for port
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		st := m.Status()
		if st.ServerOK == "open" || st.Container == "running" {
			return st, nil
		}
		time.Sleep(2 * time.Second)
	}
	return m.Status(), nil
}

func (m *Manager) Stop(ctx context.Context) (Status, error) {
	if m.composeFile != "" {
		_ = run(ctx, "docker", "compose", "-f", m.composeFile, "stop")
	} else {
		_ = run(ctx, "docker", "stop", m.container)
	}
	// leave Xvfb running (shared by host browsers); only stop if ?full=1 later
	return m.Status(), nil
}

func (m *Manager) Restart(ctx context.Context) (Status, error) {
	_, _ = m.Stop(ctx)
	return m.Start(ctx)
}

// InstallBrowsers runs playwright install chromium (user scope).
func (m *Manager) InstallBrowsers(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "npx", "--yes", "playwright@1.61.1", "install", "chromium")
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (m *Manager) browsersPath() string {
	if p := m.cfg.Sessions.PlaywrightBrowsersPath; p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	if h := os.Getenv("HOME"); h != "" {
		home = h
	}
	return filepath.Join(home, ".cache", "ms-playwright")
}

func (m *Manager) containerState() string {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", m.container).CombinedOutput()
	if err != nil {
		s := strings.TrimSpace(string(out))
		if strings.Contains(s, "No such object") || s == "" {
			return "missing"
		}
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func unitState(name string) string {
	out, err := exec.Command("systemctl", "is-active", name).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			return "missing"
		}
		return s // inactive, failed, etc.
	}
	return s
}

func probeDisplay(display string) string {
	if display == "" {
		return "unset"
	}
	if p, err := exec.LookPath("xdpyinfo"); err == nil {
		if exec.Command(p, "-display", display).Run() == nil {
			return "active"
		}
		return "down"
	}
	if strings.HasPrefix(display, ":") {
		num := strings.TrimPrefix(display, ":")
		if _, err := os.Stat("/tmp/.X" + num + "-lock"); err == nil {
			return "active"
		}
		return "down"
	}
	return "unknown"
}

func probePort(server string) string {
	if server == "" {
		return "unset"
	}
	// ws://host:port or host:port
	s := server
	s = strings.TrimPrefix(s, "ws://")
	s = strings.TrimPrefix(s, "wss://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if !strings.Contains(s, ":") {
		s = s + ":9333"
	}
	conn, err := net.DialTimeout("tcp", s, 2*time.Second)
	if err != nil {
		return "closed"
	}
	_ = conn.Close()
	return "open"
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func summarize(st Status) string {
	parts := []string{}
	if st.DisplayOK == "active" {
		parts = append(parts, "display ok")
	} else {
		parts = append(parts, "display "+st.DisplayOK)
	}
	parts = append(parts, "xvfb="+st.Xvfb, "container="+st.Container, "server="+st.ServerOK)
	return strings.Join(parts, ", ")
}
