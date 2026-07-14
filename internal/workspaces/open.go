package workspaces

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// OpenRequest asks for editor remote-open helpers for a workspace path.
type OpenRequest struct {
	// Cwd is relative to workspace_root (or ".").
	Cwd string `json:"cwd"`
	// Editor: cursor | zed | vscode | code | auto (default auto = all + optional launch).
	Editor string `json:"editor,omitempty"`
	// Launch tries to exec a local editor binary on the agents host (optional).
	Launch bool `json:"launch,omitempty"`
}

// OpenResponse is copy-pasteable commands + optional launch result.
type OpenResponse struct {
	Cwd      string            `json:"cwd"`
	Abs      string            `json:"abs"`
	SSHHost  string            `json:"ssh_host,omitempty"`
	Commands map[string]string `json:"commands"`
	// Editors lists which local binaries were found on PATH.
	Editors []string `json:"editors,omitempty"`
	// Launched is set when Launch succeeded (editor key).
	Launched    string `json:"launched,omitempty"`
	LaunchError string `json:"launch_error,omitempty"`
}

// Open builds remote/local open commands for a workspace.
func Open(cfg *config.Config, req OpenRequest) (*OpenResponse, error) {
	cwd := strings.TrimSpace(req.Cwd)
	if cwd == "" {
		cwd = cfg.DefaultCwd
	}
	if cwd == "" {
		cwd = "."
	}
	abs, err := pathallow.Resolve(cfg.WorkspaceRoot, cwd, cfg.Allow.Paths)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("cwd not a directory: %s", cwd)
	}
	// Prefer slash form for URIs
	absSlash := filepath.ToSlash(abs)
	host := strings.TrimSpace(cfg.Sessions.SSHHost)
	if host == "" {
		host = "agents"
	}
	// Strip user@ if present for vscode-remote host label
	sshRemote := host
	if i := strings.LastIndex(host, "@"); i >= 0 {
		sshRemote = host[i+1:]
	}

	cmds := map[string]string{
		// Run on your laptop (SSH remote extensions)
		"cursor_remote": fmt.Sprintf(
			"cursor --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		),
		"vscode_remote": fmt.Sprintf(
			"code --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		),
		"zed_remote": fmt.Sprintf("zed 'ssh://%s%s'", host, absSlash),
		// Run on the agents host (if GUI/editor installed there)
		"cursor_local": fmt.Sprintf("cursor %q", abs),
		"zed_local":    fmt.Sprintf("zed %q", abs),
		"vscode_local": fmt.Sprintf("code %q", abs),
		// Plain SSH shell in that dir
		"ssh": fmt.Sprintf("ssh -t %s -- %s", host, shellQuote("cd "+abs+" && exec ${SHELL:-bash} -l")),
	}

	found := detectEditors()
	out := &OpenResponse{
		Cwd:      cwd,
		Abs:      abs,
		SSHHost:  host,
		Commands: cmds,
		Editors:  found,
	}

	if req.Launch {
		ed := strings.ToLower(strings.TrimSpace(req.Editor))
		if ed == "" || ed == "auto" {
			// prefer cursor, then zed, then code
			for _, c := range []string{"cursor", "zed", "code"} {
				if hasEditor(found, c) {
					ed = c
					break
				}
			}
		}
		if ed == "" {
			out.LaunchError = "no local editor binary on PATH (cursor/zed/code)"
			return out, nil
		}
		bin, args := localLaunch(ed, abs)
		if bin == "" {
			out.LaunchError = fmt.Sprintf("unknown editor %q", ed)
			return out, nil
		}
		cmd := exec.Command(bin, args...)
		cmd.Dir = abs
		if err := cmd.Start(); err != nil {
			out.LaunchError = err.Error()
			return out, nil
		}
		// Detach: don't wait
		_ = cmd.Process.Release()
		out.Launched = ed
	}
	return out, nil
}

func shellQuote(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'"'"'`) + `'`
}

func detectEditors() []string {
	var out []string
	for _, name := range []string{"cursor", "zed", "code", "cursor-agent"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			out = append(out, name)
		}
	}
	return out
}

func hasEditor(list []string, name string) bool {
	for _, e := range list {
		if e == name {
			return true
		}
	}
	return false
}

func localLaunch(editor, abs string) (bin string, args []string) {
	switch strings.ToLower(editor) {
	case "cursor":
		if p, err := exec.LookPath("cursor"); err == nil {
			return p, []string{abs}
		}
	case "zed":
		if p, err := exec.LookPath("zed"); err == nil {
			return p, []string{abs}
		}
	case "vscode", "code", "vs code":
		if p, err := exec.LookPath("code"); err == nil {
			return p, []string{abs}
		}
	}
	return "", nil
}
