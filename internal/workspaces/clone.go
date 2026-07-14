package workspaces

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// CloneRequest creates a new project under workspace_root via git clone or gh fork.
type CloneRequest struct {
	// URL: full git URL, scp-like git@host:path, or GitHub shorthand owner/repo
	URL string `json:"url"`
	// Name: directory under workspace_root (optional; derived from repo name)
	Name string `json:"name,omitempty"`
	// Branch: optional checkout after clone
	Branch string `json:"branch,omitempty"`
	// Depth: shallow clone depth (0 = full history)
	Depth int `json:"depth,omitempty"`
	// Fork: use `gh repo fork --clone` (GitHub only). Falls back to clone if gh missing.
	Fork bool `json:"fork,omitempty"`
}

// CloneResult is returned after a successful clone/fork.
type CloneResult struct {
	OK      bool   `json:"ok"`
	Cwd     string `json:"cwd"` // relative to workspace_root
	Abs     string `json:"abs"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Forked  bool   `json:"forked,omitempty"`
	Command string `json:"command,omitempty"` // what we ran (for logs/UI)
	Output  string `json:"output,omitempty"`
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Clone clones (or forks) a git repo into an allowlisted path under workspace_root.
func Clone(cfg *config.Config, req CloneRequest) (*CloneResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH")
	}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return nil, fmt.Errorf("url is required")
	}
	gitURL, nameHint, err := normalizeGitURL(raw)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = nameHint
	}
	name = strings.TrimSuffix(name, ".git")
	if !safeName.MatchString(name) {
		return nil, fmt.Errorf("invalid directory name %q (use letters, numbers, . _ -)", name)
	}
	if name == "." || name == ".." {
		return nil, fmt.Errorf("invalid directory name")
	}

	// Must resolve under workspace + allowlist before we touch the filesystem.
	abs, err := pathallow.Resolve(cfg.WorkspaceRoot, name, cfg.Allow.Paths)
	if err != nil {
		return nil, fmt.Errorf("destination not allowed: %w", err)
	}
	if st, err := os.Stat(abs); err == nil {
		if st.IsDir() {
			// allow only empty dir
			ents, _ := os.ReadDir(abs)
			if len(ents) > 0 {
				return nil, fmt.Errorf("destination already exists and is not empty: %s", name)
			}
		} else {
			return nil, fmt.Errorf("destination exists and is not a directory: %s", name)
		}
	}

	root, err := filepath.Abs(cfg.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	// Ensure parent of abs exists (workspace root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	branch := strings.TrimSpace(req.Branch)
	depth := req.Depth
	forked := false
	var cmd *exec.Cmd
	var cmdline string
	var out []byte

	if req.Fork {
		if _, err := exec.LookPath("gh"); err != nil {
			return nil, fmt.Errorf("fork requires gh CLI on PATH")
		}
		ghRepo := ghRepoSpec(raw, gitURL)
		// Fork on GitHub (ok if already forked), then clone the caller's fork into abs.
		forkCmd := exec.Command("gh", "repo", "fork", ghRepo, "--clone=false")
		forkCmd.Dir = root
		forkCmd.Env = os.Environ()
		fo, ferr := forkCmd.CombinedOutput()
		// Ignore "already exists" style errors — we still try to clone the fork.
		_ = fo
		_ = ferr

		loginOut, lerr := exec.Command("gh", "api", "user", "-q", ".login").CombinedOutput()
		if lerr != nil {
			return nil, fmt.Errorf("gh auth required for fork: %w (%s)", lerr, strings.TrimSpace(string(loginOut)))
		}
		user := strings.TrimSpace(string(loginOut))
		repoOnly := baseName(ghRepo)
		forkSpec := user + "/" + repoOnly
		// Prefer gh repo clone (uses user credentials/SSH as configured).
		cmd = exec.Command("gh", "repo", "clone", forkSpec, abs)
		if branch != "" {
			cmd = exec.Command("gh", "repo", "clone", forkSpec, abs, "--", "-b", branch)
		}
		cmd.Dir = root
		cmd.Env = os.Environ()
		cmdline = strings.Join(cmd.Args, " ")
		out, err = runWithTimeout(cmd, 10*time.Minute)
		if err != nil {
			_ = os.RemoveAll(abs)
			return nil, fmt.Errorf("clone fork %s failed: %w (%s)", forkSpec, err, strings.TrimSpace(string(out)))
		}
		forked = true
	} else {
		args := []string{"clone"}
		if depth > 0 {
			args = append(args, "--depth", fmt.Sprintf("%d", depth))
		}
		if branch != "" {
			args = append(args, "--branch", branch)
		}
		args = append(args, "--", gitURL, abs)
		cmd = exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = os.Environ()
		cmdline = "git " + strings.Join(args, " ")
		out, err = runWithTimeout(cmd, 10*time.Minute)
		if err != nil {
			_ = os.RemoveAll(abs)
			return nil, fmt.Errorf("git clone failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("clone finished but directory missing: %s", name)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		rel = name
	}
	rel = filepath.ToSlash(rel)

	return &CloneResult{
		OK:      true,
		Cwd:     rel,
		Abs:     abs,
		URL:     gitURL,
		Name:    name,
		Forked:  forked,
		Command: cmdline,
		Output:  trimOut(string(out), 2000),
	}, nil
}

func runWithTimeout(cmd *exec.Cmd, d time.Duration) ([]byte, error) {
	// simple: CombinedOutput with process kill via timer
	done := make(chan error, 1)
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		done <- err
	}()
	select {
	case err := <-done:
		return out, err
	case <-time.After(d):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return out, fmt.Errorf("timed out after %s", d)
	}
}

func trimOut(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// normalizeGitURL returns a cloneable URL and a suggested directory name.
func normalizeGitURL(raw string) (gitURL, dirName string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "/")

	// scp-like: git@github.com:owner/repo.git
	if strings.HasPrefix(raw, "git@") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			return "", "", fmt.Errorf("invalid git SSH URL")
		}
		dirName = baseName(parts[1])
		return raw, dirName, nil
	}

	// ssh://git@host/path
	if strings.HasPrefix(raw, "ssh://") {
		u, e := url.Parse(raw)
		if e != nil {
			return "", "", fmt.Errorf("invalid ssh URL: %w", e)
		}
		dirName = baseName(u.Path)
		return raw, dirName, nil
	}

	// GitHub shorthand: owner/repo (no path traversal)
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`, raw); matched {
		if strings.Contains(raw, "..") {
			return "", "", fmt.Errorf("invalid repo shorthand")
		}
		dirName = baseName(raw)
		return "https://github.com/" + raw + ".git", dirName, nil
	}

	// https:// or http://
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		u, e := url.Parse(raw)
		if e != nil {
			return "", "", fmt.Errorf("invalid URL: %w", e)
		}
		if u.Host == "" {
			return "", "", fmt.Errorf("invalid URL host")
		}
		// strip query/fragment
		u.RawQuery = ""
		u.Fragment = ""
		dirName = baseName(u.Path)
		if dirName == "" {
			return "", "", fmt.Errorf("could not derive repo name from URL")
		}
		return u.String(), dirName, nil
	}

	// github.com/owner/repo without scheme
	if strings.HasPrefix(raw, "github.com/") || strings.HasPrefix(raw, "gitlab.com/") || strings.HasPrefix(raw, "bitbucket.org/") {
		return normalizeGitURL("https://" + raw)
	}

	return "", "", fmt.Errorf("unrecognized git URL %q (try https://…, git@…, or owner/repo)", raw)
}

func baseName(p string) string {
	p = strings.TrimSuffix(p, ".git")
	p = strings.Trim(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

func ghRepoSpec(raw, gitURL string) string {
	// Prefer owner/repo form for gh
	raw = strings.TrimSpace(raw)
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`, raw); matched {
		return raw
	}
	// parse from URL
	u := gitURL
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimPrefix(u, "https://github.com/")
	u = strings.TrimPrefix(u, "http://github.com/")
	u = strings.TrimPrefix(u, "git@github.com:")
	u = strings.TrimPrefix(u, "ssh://git@github.com/")
	u = strings.Trim(u, "/")
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`, u); matched {
		return u
	}
	return raw
}
