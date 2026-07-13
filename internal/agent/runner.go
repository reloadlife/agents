package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/reloadlife/agents/internal/config"
)

// Run starts the agent CLI in cwd with prompt, writing combined output to w.
// Returns process exit code.
func Run(ctx context.Context, cfg config.AgentConfig, agentName, cwd, prompt string, w io.Writer) (int, error) {
	if cfg.Bin == "" {
		return 1, fmt.Errorf("agent %q has empty bin", agentName)
	}
	bin, err := exec.LookPath(cfg.Bin)
	if err != nil {
		return 1, fmt.Errorf("agent binary %q not found: %w", cfg.Bin, err)
	}

	// Jobs are print/API mode only. Interactive TTY is internal/session (tmux).
	args := append([]string{}, cfg.PrintArgs...)
	if len(args) == 0 {
		args = append([]string{}, cfg.Args...)
	}
	args = append(args, buildPromptArgs(agentName, cfg, prompt)...)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = cwd
	cmd.Env = cleanEnv()
	cmd.Stdout = w
	cmd.Stderr = w
	// separate process group for cancel
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 1, err
	}

	// if context cancelled, kill process group
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
		case <-done:
		}
	}()

	err = cmd.Wait()
	close(done)
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	// context cancel
	if ctx.Err() != nil {
		return 130, ctx.Err()
	}
	return 1, err
}

func buildPromptArgs(agentName string, cfg config.AgentConfig, prompt string) []string {
	name := strings.ToLower(agentName)
	// mock uses echo: args already have prefix; append prompt as words is fine as single arg
	switch name {
	case "mock":
		return []string{prompt}
	case "claude":
		// print-mode jobs only — prefer PrintArgs
		base := cfg.PrintArgs
		if len(base) == 0 {
			base = cfg.Args
		}
		if hasFlag(base, "-p") || hasFlag(base, "--print") {
			return []string{prompt}
		}
		return []string{"-p", prompt}
	case "codex":
		// codex exec "prompt"
		return []string{prompt}
	case "opencode":
		return []string{prompt}
	case "grok":
		// best-effort; grok CLI shapes vary — pass as trailing args
		return []string{prompt}
	default:
		return []string{prompt}
	}
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func cleanEnv() []string {
	// keep a minimal env so agent CLIs still work (PATH, HOME, auth dirs)
	keep := []string{
		"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		// agent auth often lives under home; allow common vars if present
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "XAI_API_KEY",
		"CLAUDE_CONFIG_DIR", "CODEX_HOME",
		"SSH_AUTH_SOCK", "GITHUB_TOKEN", "GH_TOKEN",
		"GOROOT", "GOPATH", "GOBIN",
		"BUN_INSTALL", "NVM_DIR", "CARGO_HOME", "RUSTUP_HOME",
		"USE_GKE_GCLOUD_AUTH_PLUGIN", "CLOUDSDK_CORE_PROJECT",
		"SOPS_AGE_KEY_FILE", "SECRET_STORE",
	}
	var out []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	// ensure PATH exists
	if _, ok := os.LookupEnv("PATH"); !ok {
		out = append(out, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	return out
}
