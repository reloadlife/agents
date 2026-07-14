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
	// Path is an optional file path relative to the resolved cwd (no escape).
	Path string `json:"path,omitempty"`
	// Line is an optional 1-based line number to open at (when path is set).
	Line int `json:"line,omitempty"`
	// Editor: cursor | zed | vscode | code | auto (default auto = all + optional launch).
	Editor string `json:"editor,omitempty"`
	// Launch tries to exec a local editor binary on the agents host (optional).
	Launch bool `json:"launch,omitempty"`
}

// OpenResponse is copy-pasteable commands + optional launch result.
type OpenResponse struct {
	Cwd  string `json:"cwd"`
	Abs  string `json:"abs"`
	// Path is the relative file path when opening a file (echo of request).
	Path string `json:"path,omitempty"`
	// Line is the requested line when > 0.
	Line     int               `json:"line,omitempty"`
	SSHHost  string            `json:"ssh_host,omitempty"`
	Commands map[string]string `json:"commands"`
	// Editors lists which local binaries were found on PATH.
	Editors []string `json:"editors,omitempty"`
	// Launched is set when Launch succeeded (editor key).
	Launched    string `json:"launched,omitempty"`
	LaunchError string `json:"launch_error,omitempty"`
}

// Open builds remote/local open commands for a workspace (and optional file).
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

	relPath := strings.TrimSpace(req.Path)
	line := req.Line
	if line < 0 {
		line = 0
	}

	// Target for editors: folder abs, or file abs under cwd (no escape).
	targetAbs := abs
	if relPath != "" {
		fileAbs, err := resolveUnderRoot(abs, relPath)
		if err != nil {
			return nil, err
		}
		targetAbs = fileAbs
	}

	// Prefer slash form for URIs
	absSlash := filepath.ToSlash(abs)
	targetSlash := filepath.ToSlash(targetAbs)
	host := strings.TrimSpace(cfg.Sessions.SSHHost)
	if host == "" {
		host = "agents"
	}
	// Strip user@ if present for vscode-remote host label
	sshRemote := host
	if i := strings.LastIndex(host, "@"); i >= 0 {
		sshRemote = host[i+1:]
	}

	cmds := buildOpenCommands(host, sshRemote, abs, absSlash, targetAbs, targetSlash, relPath != "", line)

	found := detectEditors()
	out := &OpenResponse{
		Cwd:      cwd,
		Abs:      abs,
		SSHHost:  host,
		Commands: cmds,
		Editors:  found,
	}
	if relPath != "" {
		out.Path = relPath
	}
	if line > 0 {
		out.Line = line
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
		bin, args := localLaunch(ed, targetAbs, line)
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

// resolveUnderRoot joins rel under root and rejects absolute paths, "..", and escapes.
func resolveUnderRoot(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return root, nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to cwd")
	}
	// Reject .. segments even after Clean would collapse them away from checks.
	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("path must not contain ..")
	}
	joined := filepath.Join(root, filepath.Clean(rel))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	// Also resolve root for fair prefix compare when root has symlinks.
	rootClean := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		rootClean = resolved
	}
	rootClean = filepath.Clean(rootClean)
	abs = filepath.Clean(abs)

	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace cwd")
	}
	return abs, nil
}

func buildOpenCommands(host, sshRemote, abs, absSlash, targetAbs, targetSlash string, isFile bool, line int) map[string]string {
	// Local goto target: path or path:line
	localGoto := targetAbs
	if isFile && line > 0 {
		localGoto = fmt.Sprintf("%s:%d", targetAbs, line)
	}
	// Remote vscode-remote resource path (with optional :line for --goto)
	remoteResource := targetSlash
	if isFile && line > 0 {
		remoteResource = fmt.Sprintf("%s:%d", targetSlash, line)
	}
	// Zed ssh URI (path:line supported)
	zedRemoteTarget := targetSlash
	if isFile && line > 0 {
		zedRemoteTarget = fmt.Sprintf("%s:%d", targetSlash, line)
	}

	cmds := map[string]string{
		// Plain SSH shell in the workspace dir
		"ssh": fmt.Sprintf("ssh -t %s -- %s", host, shellQuote("cd "+abs+" && exec ${SHELL:-bash} -l")),
	}

	if isFile {
		// File-aware remote URIs
		cmds["cursor_remote"] = fmt.Sprintf(
			"cursor --goto 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, remoteResource,
		)
		cmds["vscode_remote"] = fmt.Sprintf(
			"code --goto 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, remoteResource,
		)
		// Also provide --file-uri form without line (useful when :line not honored)
		cmds["cursor_remote_file"] = fmt.Sprintf(
			"cursor --file-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, targetSlash,
		)
		cmds["vscode_remote_file"] = fmt.Sprintf(
			"code --file-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, targetSlash,
		)
		// Folder open still useful to land the remote workspace
		cmds["cursor_remote_folder"] = fmt.Sprintf(
			"cursor --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		)
		cmds["vscode_remote_folder"] = fmt.Sprintf(
			"code --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		)
		cmds["zed_remote"] = fmt.Sprintf("zed 'ssh://%s%s'", host, zedRemoteTarget)
		// Local CLIs: -g / path:line when supported
		if line > 0 {
			cmds["cursor_local"] = fmt.Sprintf("cursor -g %q", localGoto)
			cmds["vscode_local"] = fmt.Sprintf("code -g %q", localGoto)
			cmds["zed_local"] = fmt.Sprintf("zed %q", localGoto)
		} else {
			cmds["cursor_local"] = fmt.Sprintf("cursor %q", targetAbs)
			cmds["vscode_local"] = fmt.Sprintf("code %q", targetAbs)
			cmds["zed_local"] = fmt.Sprintf("zed %q", targetAbs)
		}
	} else {
		cmds["cursor_remote"] = fmt.Sprintf(
			"cursor --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		)
		cmds["vscode_remote"] = fmt.Sprintf(
			"code --folder-uri 'vscode-remote://ssh-remote+%s%s'",
			sshRemote, absSlash,
		)
		cmds["zed_remote"] = fmt.Sprintf("zed 'ssh://%s%s'", host, absSlash)
		cmds["cursor_local"] = fmt.Sprintf("cursor %q", abs)
		cmds["zed_local"] = fmt.Sprintf("zed %q", abs)
		cmds["vscode_local"] = fmt.Sprintf("code %q", abs)
	}
	return cmds
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

func localLaunch(editor, path string, line int) (bin string, args []string) {
	gotoArg := path
	if line > 0 {
		gotoArg = fmt.Sprintf("%s:%d", path, line)
	}
	switch strings.ToLower(editor) {
	case "cursor":
		if p, err := exec.LookPath("cursor"); err == nil {
			if line > 0 {
				return p, []string{"-g", gotoArg}
			}
			return p, []string{path}
		}
	case "zed":
		if p, err := exec.LookPath("zed"); err == nil {
			return p, []string{gotoArg}
		}
	case "vscode", "code", "vs code":
		if p, err := exec.LookPath("code"); err == nil {
			if line > 0 {
				return p, []string{"-g", gotoArg}
			}
			return p, []string{path}
		}
	}
	return "", nil
}
