package workspaces

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/redact"
)

// CommitRequest stages files and creates a local git commit (never pushes).
type CommitRequest struct {
	// Cwd is relative to workspace_root.
	Cwd string `json:"cwd"`
	// Message is the commit message (required, non-empty).
	Message string `json:"message"`
	// Paths are paths relative to cwd (or repo) to `git add`. Ignored when All is true.
	Paths []string `json:"paths,omitempty"`
	// All runs `git add -A` before commit.
	All bool `json:"all,omitempty"`
}

// CommitResult is returned after a successful local commit.
type CommitResult struct {
	OK      bool   `json:"ok"`
	Cwd     string `json:"cwd"`
	Commit  string `json:"commit"`
	Branch  string `json:"branch"`
	Message string `json:"message"`
}

// PullRequestRequest creates a GitHub PR via `gh pr create`.
// Host must have gh authenticated. Does not implement push here.
type PullRequestRequest struct {
	// Cwd is relative to workspace_root.
	Cwd string `json:"cwd"`
	// Title is the PR title (required).
	Title string `json:"title"`
	// Body is the PR description.
	Body string `json:"body,omitempty"`
	// Base branch (optional; gh default when empty).
	Base string `json:"base,omitempty"`
	// Draft creates a draft PR.
	Draft bool `json:"draft,omitempty"`
}

// PullRequestResult is returned after a successful PR create.
type PullRequestResult struct {
	OK     bool   `json:"ok"`
	URL    string `json:"url"`
	Number int    `json:"number,omitempty"`
	Cwd    string `json:"cwd"`
}

const (
	gitWriteTimeout = 2 * time.Minute
	ghPRTimeout     = 3 * time.Minute
	errOutCap       = 1500
)

var rePRNumber = regexp.MustCompile(`(?i)/pull/(\d+)`)

// Commit stages (optional) and creates a local commit. NEVER pushes.
func Commit(cfg *config.Config, req CommitRequest) (*CommitResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH")
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return nil, fmt.Errorf("message is required")
	}

	abs, rel, err := resolveWorkspaceCwd(cfg, req.Cwd)
	if err != nil {
		return nil, fmt.Errorf("cwd not allowed: %w", err)
	}
	repoRoot, err := gitShowToplevel(abs)
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}
	if err := ensureUnderWorkspace(cfg, repoRoot); err != nil {
		return nil, err
	}

	if req.All {
		if out, err := gitRun(repoRoot, gitWriteTimeout, "add", "-A"); err != nil {
			return nil, fmt.Errorf("git add -A failed: %s", safeErrOut(out, err))
		}
	} else if len(req.Paths) > 0 {
		var repoPaths []string
		for _, p := range req.Paths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			rp, err := resolvePathUnderRepo(repoRoot, abs, p)
			if err != nil {
				return nil, err
			}
			repoPaths = append(repoPaths, rp)
		}
		if len(repoPaths) == 0 {
			return nil, fmt.Errorf("paths is empty")
		}
		args := append([]string{"add", "--"}, repoPaths...)
		if out, err := gitRun(repoRoot, gitWriteTimeout, args...); err != nil {
			return nil, fmt.Errorf("git add failed: %s", safeErrOut(out, err))
		}
	}

	// Refuse empty commits (no staged changes).
	// git diff --cached --quiet: exit 0 = clean, exit 1 = has staged diffs.
	if code, out, err := gitRunCode(repoRoot, 30*time.Second, "diff", "--cached", "--quiet"); err != nil && code < 0 {
		return nil, fmt.Errorf("git diff --cached failed: %s", safeErrOut(out, err))
	} else if code == 0 {
		return nil, fmt.Errorf("nothing to commit (no staged changes; pass paths or all=true)")
	} else if code != 1 {
		return nil, fmt.Errorf("git diff --cached failed: %s", safeErrOut(out, err))
	}

	if out, err := gitRun(repoRoot, gitWriteTimeout, "commit", "-m", msg); err != nil {
		return nil, fmt.Errorf("git commit failed: %s", safeErrOut(out, err))
	}

	hash, err := gitOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD failed: %s", capRedact(err.Error(), errOutCap))
	}
	branch, err := gitOutput(repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse branch failed: %s", capRedact(err.Error(), errOutCap))
	}

	return &CommitResult{
		OK:      true,
		Cwd:     filepath.ToSlash(rel),
		Commit:  strings.TrimSpace(hash),
		Branch:  strings.TrimSpace(branch),
		Message: msg,
	}, nil
}

// CreatePullRequest runs `gh pr create` in the resolved workspace cwd.
// Does not auto-push from this package; the branch must already exist on the remote
// (or gh may push depending on user git/gh config — we never run git push here).
func CreatePullRequest(cfg *config.Config, req PullRequestRequest) (*PullRequestResult, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found on PATH")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	abs, rel, err := resolveWorkspaceCwd(cfg, req.Cwd)
	if err != nil {
		return nil, fmt.Errorf("cwd not allowed: %w", err)
	}
	if _, err := gitShowToplevel(abs); err != nil {
		return nil, fmt.Errorf("not a git repository")
	}

	args := []string{"pr", "create", "--title", title, "--body", req.Body}
	if base := strings.TrimSpace(req.Base); base != "" {
		args = append(args, "--base", base)
	}
	if req.Draft {
		args = append(args, "--draft")
	}

	out, err := ghRun(abs, ghPRTimeout, args...)
	if err != nil {
		return nil, fmt.Errorf("gh pr create failed: %s", safeErrOut(out, err))
	}

	url, number := parsePRCreateOutput(string(out))
	if url == "" {
		trimmed := strings.TrimSpace(string(out))
		return nil, fmt.Errorf("gh pr create produced no URL: %s", capRedact(trimmed, errOutCap))
	}

	return &PullRequestResult{
		OK:     true,
		URL:    url,
		Number: number,
		Cwd:    filepath.ToSlash(rel),
	}, nil
}

// gitRun runs git -C dir with timeout; returns combined output.
func gitRun(dir string, d time.Duration, args ...string) ([]byte, error) {
	_, out, err := gitRunCode(dir, d, args...)
	return out, err
}

// gitRunCode returns process exit code (-1 if non-exit error / timeout).
func gitRunCode(dir string, d time.Duration, args ...string) (code int, out []byte, err error) {
	cmd := gitCmd(dir, args...)
	done := make(chan error, 1)
	go func() {
		out, err = cmd.CombinedOutput()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			return 0, out, nil
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), out, err
		}
		return -1, out, err
	case <-time.After(d):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return -1, out, fmt.Errorf("timed out after %s", d)
	}
}

func ghRun(dir string, d time.Duration, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
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

func safeErrOut(out []byte, err error) string {
	msg := strings.TrimSpace(string(out))
	if msg == "" && err != nil {
		msg = err.Error()
	} else if err != nil && msg != "" && !strings.Contains(msg, err.Error()) {
		// keep git/gh stderr primary; append short err if useful
		if ee, ok := err.(*exec.ExitError); ok {
			msg = fmt.Sprintf("%s (exit %d)", msg, ee.ExitCode())
		}
	}
	return capRedact(msg, errOutCap)
}

func capRedact(s string, n int) string {
	s = redact.Line(s)
	s = strings.TrimSpace(s)
	if n > 0 && len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func parsePRCreateOutput(raw string) (url string, number int) {
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, "https://") || strings.HasPrefix(f, "http://") {
				return f, prNumberFromURL(f)
			}
		}
	}
	return "", 0
}

func prNumberFromURL(u string) int {
	m := rePRNumber.FindStringSubmatch(u)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
