package workspaces

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/reloadlife/agents/internal/pathallow"
)

// WorktreeResult is returned after creating an isolated session worktree.
type WorktreeResult struct {
	// Rel path under workspace_root (session.cwd)
	Path string
	// Absolute path of the worktree checkout
	Abs string
	// Branch checked out in the worktree
	Branch string
	// BaseCwd is the original relative cwd (before isolation)
	BaseCwd string
	// RepoAbs is the git toplevel used to create the worktree
	RepoAbs string
}

var safeBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// ShortSessionID returns a filesystem/branch-safe short form of a session id
// (e.g. "s_01ARZ3NDEK…" → "01arz3ndek").
func ShortSessionID(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	id = strings.TrimPrefix(id, "s_")
	id = strings.TrimPrefix(id, "S_")
	if id == "" {
		return "unknown"
	}
	if len(id) > 10 {
		id = id[:10]
	}
	return strings.ToLower(id)
}

// SanitizeBranch validates a user-provided git branch name.
func SanitizeBranch(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("branch name is empty")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid branch name")
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return "", fmt.Errorf("invalid branch name")
	}
	if !safeBranch.MatchString(name) {
		return "", fmt.Errorf("invalid branch name %q (use letters, numbers, . _ / -)", name)
	}
	if len(name) > 200 {
		return "", fmt.Errorf("branch name too long")
	}
	return name, nil
}

// IsGitWorkTree reports whether absPath is inside a git working tree.
func IsGitWorkTree(absPath string) bool {
	if strings.TrimSpace(absPath) == "" {
		return false
	}
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// RepoRoot returns the absolute git toplevel for absPath.
func RepoRoot(absPath string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH")
	}
	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(out)))
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("empty git toplevel")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// CreateSessionWorktree creates a new git worktree for an agents session.
//
// Path preference:
//  1. <repo>/.agents/worktrees/<short-id> when the repo root is under workspaceRoot
//  2. <workspaceRoot>/.agents/worktrees/<short-id> otherwise
//
// Branch defaults to agents/<short-id> unless branch is provided.
// baseCwdRel is the session cwd relative to workspace_root (before isolation).
func CreateSessionWorktree(workspaceRoot, baseCwdRel, baseCwdAbs, sessionID, branch string, allowPaths []string) (*WorktreeResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH")
	}
	baseCwdAbs = filepath.Clean(baseCwdAbs)
	if !IsGitWorkTree(baseCwdAbs) {
		return nil, fmt.Errorf("cwd is not inside a git repository")
	}
	repoAbs, err := RepoRoot(baseCwdAbs)
	if err != nil {
		return nil, err
	}

	wsRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(wsRoot); err == nil {
		wsRoot = resolved
	}
	wsRoot = filepath.Clean(wsRoot)

	short := ShortSessionID(sessionID)
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "agents/" + short
	} else {
		branch, err = SanitizeBranch(branch)
		if err != nil {
			return nil, err
		}
	}

	// Prefer worktree under the repo when that stays inside the workspace.
	var worktreeRel string
	if underWorkspace(wsRoot, repoAbs) {
		repoRel, rerr := filepath.Rel(wsRoot, repoAbs)
		if rerr != nil {
			return nil, rerr
		}
		worktreeRel = filepath.ToSlash(filepath.Join(repoRel, ".agents", "worktrees", short))
	} else {
		worktreeRel = filepath.ToSlash(filepath.Join(".agents", "worktrees", short))
	}
	if worktreeRel == "." || strings.HasPrefix(worktreeRel, "..") {
		return nil, fmt.Errorf("worktree path escapes workspace")
	}

	worktreeAbs, err := pathallow.Resolve(wsRoot, worktreeRel, allowPaths)
	if err != nil {
		return nil, fmt.Errorf("worktree path not allowed: %w", err)
	}
	// Extra safety: never point worktree at the repo root or workspace root itself.
	if worktreeAbs == repoAbs || worktreeAbs == wsRoot || worktreeAbs == baseCwdAbs {
		return nil, fmt.Errorf("refusing to create worktree at repository root")
	}

	if st, err := os.Stat(worktreeAbs); err == nil {
		if st.IsDir() {
			ents, _ := os.ReadDir(worktreeAbs)
			if len(ents) > 0 {
				return nil, fmt.Errorf("worktree path already exists: %s", worktreeRel)
			}
		} else {
			return nil, fmt.Errorf("worktree path exists and is not a directory: %s", worktreeRel)
		}
	}

	if err := os.MkdirAll(filepath.Dir(worktreeAbs), 0o755); err != nil {
		return nil, err
	}

	// Create a new branch from HEAD of the base checkout's repo.
	// -B resets the branch if it already exists (e.g. stale session leftover).
	args := []string{"worktree", "add", "-B", branch, worktreeAbs}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoAbs
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(worktreeAbs)
		return nil, fmt.Errorf("git worktree add failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Re-resolve after creation (symlinks, allowlist).
	finalAbs, err := pathallow.Resolve(wsRoot, worktreeRel, allowPaths)
	if err != nil {
		_ = RemoveWorktree(repoAbs, worktreeAbs)
		return nil, fmt.Errorf("worktree path not allowed after create: %w", err)
	}
	if !underWorkspace(wsRoot, finalAbs) {
		_ = RemoveWorktree(repoAbs, worktreeAbs)
		return nil, fmt.Errorf("worktree path escapes workspace root")
	}

	baseRel := strings.TrimSpace(baseCwdRel)
	if baseRel == "" {
		baseRel = "."
	}

	return &WorktreeResult{
		Path:    filepath.ToSlash(worktreeRel),
		Abs:     finalAbs,
		Branch:  branch,
		BaseCwd: filepath.ToSlash(baseRel),
		RepoAbs: repoAbs,
	}, nil
}

// RemoveWorktree best-effort removes a linked worktree. It never removes the
// primary repository checkout (where .git is a directory).
func RemoveWorktree(repoAbs, worktreeAbs string) error {
	worktreeAbs = filepath.Clean(strings.TrimSpace(worktreeAbs))
	if worktreeAbs == "" || worktreeAbs == "." || worktreeAbs == "/" {
		return fmt.Errorf("refusing to remove empty/invalid worktree path")
	}
	repoAbs = filepath.Clean(strings.TrimSpace(repoAbs))
	if repoAbs != "" && worktreeAbs == repoAbs {
		return fmt.Errorf("refusing to remove primary repository")
	}

	// Linked worktrees have a .git *file*; the main checkout has a .git directory.
	gitMeta := filepath.Join(worktreeAbs, ".git")
	st, err := os.Lstat(gitMeta)
	if err != nil {
		// Already gone — treat as success for best-effort cleanup.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("refusing to remove primary worktree ( .git is a directory )")
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH")
	}

	// Prefer running from the main repo when known.
	dir := repoAbs
	if dir == "" || !isDir(dir) {
		// Fall back: common dir from the worktree itself.
		cmd := exec.Command("git", "-C", worktreeAbs, "rev-parse", "--git-common-dir")
		out, cerr := cmd.CombinedOutput()
		if cerr != nil {
			return fmt.Errorf("cannot locate git common dir: %w (%s)", cerr, strings.TrimSpace(string(out)))
		}
		common := strings.TrimSpace(string(out))
		if !filepath.IsAbs(common) {
			common = filepath.Join(worktreeAbs, common)
		}
		// common-dir is usually <repo>/.git — parent is the main tree or bare repo root.
		dir = filepath.Clean(filepath.Join(common, ".."))
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", worktreeAbs)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Directory may already be half-removed; try prune + rmdir.
		_ = exec.Command("git", "-C", dir, "worktree", "prune").Run()
		if st, serr := os.Stat(worktreeAbs); serr == nil && st.IsDir() {
			if rerr := os.RemoveAll(worktreeAbs); rerr != nil {
				return fmt.Errorf("git worktree remove failed: %w (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		_ = exec.Command("git", "-C", dir, "worktree", "prune").Run()
		return nil
	}
	_ = exec.Command("git", "-C", dir, "worktree", "prune").Run()
	return nil
}

func underWorkspace(wsRoot, abs string) bool {
	wsRoot = filepath.Clean(wsRoot)
	abs = filepath.Clean(abs)
	if abs == wsRoot {
		return true
	}
	return strings.HasPrefix(abs, wsRoot+string(filepath.Separator))
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
