package workspaces

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// Cap unified-diff payload so a huge change does not blow API clients.
const maxDiffBytes = 1572864 // 1.5 MiB

// StatusResult is read-only git worktree status for a workspace cwd.
type StatusResult struct {
	Cwd      string         `json:"cwd"`
	Abs      string         `json:"abs"`
	IsRepo   bool           `json:"is_repo"`
	Root     string         `json:"root,omitempty"`
	Branch   string         `json:"branch,omitempty"`
	Upstream string         `json:"upstream,omitempty"`
	Ahead    *int           `json:"ahead,omitempty"`
	Behind   *int           `json:"behind,omitempty"`
	Head     string         `json:"head,omitempty"`
	Dirty    bool           `json:"dirty"`
	Files    []FileStatus   `json:"files"`
	Summary  StatusSummary  `json:"summary"`
}

// FileStatus is one path from porcelain status (+ optional numstat).
type FileStatus struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Staged    bool   `json:"staged"`
	Unstaged  bool   `json:"unstaged"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
}

// StatusSummary aggregates file change kinds.
type StatusSummary struct {
	Staged     int `json:"staged"`
	Unstaged   int `json:"unstaged"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
}

// DiffResult is a unified diff (possibly truncated).
type DiffResult struct {
	Cwd       string `json:"cwd"`
	Path      string `json:"path,omitempty"`
	Staged    bool   `json:"staged"`
	Base      string `json:"base"`
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary,omitempty"`
}

// FileBlobResult is file content at a ref (default HEAD).
type FileBlobResult struct {
	Cwd      string `json:"cwd"`
	Path     string `json:"path"`
	Ref      string `json:"ref"`
	Content  string `json:"content"`
	Binary   bool   `json:"binary,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
	Size     int    `json:"size"`
	Truncated bool  `json:"truncated,omitempty"`
}

// Status returns git status for cwd (relative to workspace_root / allowlist).
// Non-repo directories return is_repo:false with empty files (no error).
func Status(cfg *config.Config, cwd string) (*StatusResult, error) {
	abs, rel, err := resolveWorkspaceCwd(cfg, cwd)
	if err != nil {
		return nil, err
	}
	out := &StatusResult{
		Cwd:   rel,
		Abs:   abs,
		Files: []FileStatus{},
	}
	if _, err := exec.LookPath("git"); err != nil {
		return out, nil
	}
	root, err := gitShowToplevel(abs)
	if err != nil || root == "" {
		return out, nil
	}
	out.IsRepo = true
	out.Root = root

	if br, err := gitOutput(abs, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		out.Branch = strings.TrimSpace(br)
		if out.Branch == "HEAD" {
			// detached
			if short, err := gitOutput(abs, "rev-parse", "--short", "HEAD"); err == nil {
				out.Branch = "HEAD detached at " + strings.TrimSpace(short)
			}
		}
	}
	if head, err := gitOutput(abs, "rev-parse", "--short", "HEAD"); err == nil {
		out.Head = strings.TrimSpace(head)
	}
	if up, err := gitOutput(abs, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		up = strings.TrimSpace(up)
		if up != "" {
			out.Upstream = up
			if ab, err := gitOutput(abs, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
				// "ahead\tbehind"
				fields := strings.Fields(strings.TrimSpace(ab))
				if len(fields) == 2 {
					if a, e1 := strconv.Atoi(fields[0]); e1 == nil {
						if b, e2 := strconv.Atoi(fields[1]); e2 == nil {
							out.Ahead = &a
							out.Behind = &b
						}
					}
				}
			}
		}
	}

	porcelain, err := gitOutput(abs, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	files := parsePorcelain(porcelain)

	// Merge numstat additions/deletions (unstaged + staged).
	adds := map[string]int{}
	dels := map[string]int{}
	mergeNumstat(adds, dels, gitOutputQuiet(abs, "diff", "--numstat"))
	mergeNumstat(adds, dels, gitOutputQuiet(abs, "diff", "--cached", "--numstat"))
	for i := range files {
		p := files[i].Path
		if n, ok := adds[p]; ok {
			files[i].Additions = n
		}
		if n, ok := dels[p]; ok {
			files[i].Deletions = n
		}
	}

	var sum StatusSummary
	for _, f := range files {
		if f.Staged {
			sum.Staged++
		}
		if isUntracked(f.Status) {
			sum.Untracked++
		} else if f.Unstaged {
			sum.Unstaged++
		}
		if isConflicted(f.Status) {
			sum.Conflicted++
		}
	}
	out.Files = files
	out.Summary = sum
	out.Dirty = len(files) > 0
	return out, nil
}

// Diff returns unified diff for the worktree (or staged index) vs base (default HEAD).
// path, when set, must stay under the git root and workspace allowlist.
func Diff(cfg *config.Config, cwd, path string, staged bool, base string) (*DiffResult, error) {
	abs, rel, err := resolveWorkspaceCwd(cfg, cwd)
	if err != nil {
		return nil, err
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "HEAD"
	}
	if err := validateGitRef(base); err != nil {
		return nil, err
	}
	out := &DiffResult{
		Cwd:    rel,
		Path:   strings.TrimSpace(path),
		Staged: staged,
		Base:   base,
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH")
	}
	root, err := gitShowToplevel(abs)
	if err != nil || root == "" {
		return nil, fmt.Errorf("not a git repository")
	}

	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if staged {
		// staged index vs base (usually HEAD)
		args = append(args, "--cached")
		if base != "HEAD" {
			args = append(args, base)
		}
	} else {
		// worktree vs base (includes unstaged; not untracked content)
		args = append(args, base)
	}

	path = strings.TrimSpace(path)
	if path != "" {
		safe, err := resolvePathUnderRepo(root, abs, path)
		if err != nil {
			return nil, err
		}
		// double-check absolute path stays under workspace root
		if err := ensureUnderWorkspace(cfg, filepath.Join(root, filepath.FromSlash(safe))); err != nil {
			return nil, err
		}
		out.Path = filepath.ToSlash(safe)
		args = append(args, "--", safe)
	}

	raw, err := gitOutputBytes(abs, args...)
	if err != nil {
		// empty tree / missing base may still return exit 0; non-zero is real failure
		// allow "diff against missing" style? return error with stderr
		return nil, fmt.Errorf("git diff: %w", err)
	}
	diff, trunc, binary := capDiff(raw, maxDiffBytes)
	out.Diff = diff
	out.Truncated = trunc
	out.Binary = binary
	return out, nil
}

// FileAtRef returns blob content for path at ref (default HEAD).
func FileAtRef(cfg *config.Config, cwd, path, ref string) (*FileBlobResult, error) {
	abs, rel, err := resolveWorkspaceCwd(cfg, cwd)
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	if err := validateGitRef(ref); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH")
	}
	root, err := gitShowToplevel(abs)
	if err != nil || root == "" {
		return nil, fmt.Errorf("not a git repository")
	}
	safe, err := resolvePathUnderRepo(root, abs, path)
	if err != nil {
		return nil, err
	}
	if err := ensureUnderWorkspace(cfg, filepath.Join(root, filepath.FromSlash(safe))); err != nil {
		// path is relative to repo; also allow if repo root is under workspace
		if err2 := ensureUnderWorkspace(cfg, root); err2 != nil {
			return nil, err
		}
	}
	out := &FileBlobResult{
		Cwd:  rel,
		Path: filepath.ToSlash(safe),
		Ref:  ref,
	}
	// git show ref:path — path must use / separators
	spec := ref + ":" + filepath.ToSlash(safe)
	raw, err := gitOutputBytes(abs, "show", spec)
	if err != nil {
		out.Missing = true
		out.Content = ""
		return out, nil
	}
	content, trunc, binary := capDiff(raw, maxDiffBytes)
	out.Content = content
	out.Truncated = trunc
	out.Binary = binary
	out.Size = len(raw)
	if trunc {
		out.Size = len(raw) // original size
	}
	return out, nil
}

func resolveWorkspaceCwd(cfg *config.Config, cwd string) (abs, rel string, err error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = cfg.DefaultCwd
	}
	if cwd == "" {
		cwd = "."
	}
	abs, err = pathallow.Resolve(cfg.WorkspaceRoot, cwd, cfg.Allow.Paths)
	if err != nil {
		return "", "", err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", "", fmt.Errorf("cwd not a directory: %s", cwd)
	}
	return abs, cwd, nil
}

func gitShowToplevel(abs string) (string, error) {
	out, err := gitOutput(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitCmd(abs string, args ...string) *exec.Cmd {
	all := append([]string{"-C", abs}, args...)
	cmd := exec.Command("git", all...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
	)
	return cmd
}

func gitOutput(abs string, args ...string) (string, error) {
	cmd := gitCmd(abs, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func gitOutputQuiet(abs string, args ...string) string {
	s, _ := gitOutput(abs, args...)
	return s
}

func gitOutputBytes(abs string, args ...string) ([]byte, error) {
	cmd := gitCmd(abs, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

func parsePorcelain(s string) []FileStatus {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.TrimSpace(s) == "" {
		return []FileStatus{}
	}
	var files []FileStatus
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		// porcelain v1: XY PATH or XY ORIG -> PATH (rename/copy)
		if len(line) < 3 {
			continue
		}
		x := line[0]
		y := line[1]
		rest := line[2:]
		if strings.HasPrefix(rest, " ") {
			rest = rest[1:]
		}
		path := rest
		if i := strings.Index(rest, " -> "); i >= 0 {
			// rename/copy: keep destination path
			path = rest[i+4:]
		}
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		st := statusCode(x, y)
		staged := x != ' ' && x != '?'
		unstaged := y != ' ' || x == '?'
		// conflict codes: U in either side, or DD/AA
		if isConflictXY(x, y) {
			staged = true
			unstaged = true
		}
		files = append(files, FileStatus{
			Path:     path,
			Status:   st,
			Staged:   staged,
			Unstaged: unstaged,
		})
	}
	return files
}

func statusCode(x, y byte) string {
	if x == '?' && y == '?' {
		return "??"
	}
	if x == '!' && y == '!' {
		return "!!"
	}
	if isConflictXY(x, y) {
		return string([]byte{x, y})
	}
	// Prefer a single letter when only one side dirty; both when MM etc.
	if x != ' ' && y != ' ' && x != '?' {
		return string([]byte{x, y})
	}
	if y != ' ' && y != '?' {
		return string([]byte{y})
	}
	if x != ' ' && x != '?' {
		return string([]byte{x})
	}
	return string([]byte{x, y})
}

func isConflictXY(x, y byte) bool {
	return x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
}

func isUntracked(st string) bool {
	return st == "??"
}

func isConflicted(st string) bool {
	if st == "" {
		return false
	}
	if strings.Contains(st, "U") {
		return true
	}
	return st == "AA" || st == "DD"
}

func mergeNumstat(adds, dels map[string]int, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// additions\tdeletions\tpath  (binary: -\t-\tpath)
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := filepath.ToSlash(parts[len(parts)-1])
		// rename form may appear as "old => new" in some versions; take last segment after =>
		if i := strings.LastIndex(path, " => "); i >= 0 {
			path = strings.TrimSpace(path[i+4:])
		}
		if parts[0] != "-" {
			if n, err := strconv.Atoi(parts[0]); err == nil {
				adds[path] += n
			}
		}
		if parts[1] != "-" {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				dels[path] += n
			}
		}
	}
}

// resolvePathUnderRepo ensures path is relative and stays under git root.
// Returns path relative to repo root using slash separators.
func resolvePathUnderRepo(repoRoot, cwdAbs, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative")
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path must not contain ..")
	}
	// Interpret path relative to requested cwd, then re-base onto repo root.
	joined := filepath.Join(cwdAbs, filepath.Clean(path))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	root := repoRoot
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	// File may not exist yet — still require under root by string prefix after clean.
	// If path doesn't exist, EvalSymlinks may fail; use non-resolved join under cwdAbs.
	if !fileExists(abs) {
		// Keep logical path under cwdAbs without symlink resolve
		abs = filepath.Clean(filepath.Join(cwdAbs, filepath.Clean(path)))
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root")
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path escapes repository root")
	}
	return rel, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func ensureUnderWorkspace(cfg *config.Config, absPath string) error {
	root, err := filepath.Abs(cfg.WorkspaceRoot)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	root = filepath.Clean(root)
	abs := filepath.Clean(absPath)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
}

// validateGitRef blocks option injection and path-like refs.
func validateGitRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid ref")
	}
	if strings.ContainsAny(ref, " \t\n\r") {
		return fmt.Errorf("invalid ref")
	}
	// Disallow path:blob forms, traversal, and shell metacharacters.
	if strings.Contains(ref, "..") || strings.Contains(ref, ":") {
		return fmt.Errorf("invalid ref")
	}
	if strings.ContainsAny(ref, ";|&$`\"'\\") {
		return fmt.Errorf("invalid ref")
	}
	return nil
}

func capDiff(raw []byte, max int) (content string, truncated, binary bool) {
	if max <= 0 {
		max = maxDiffBytes
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		binary = true
		// still return a short marker rather than dumping binary
		msg := "[binary content omitted]\n"
		if len(raw) > max {
			return msg, true, true
		}
		return msg, false, true
	}
	if len(raw) > max {
		return string(raw[:max]) + "\n…[truncated]\n", true, false
	}
	return string(raw), false, false
}
