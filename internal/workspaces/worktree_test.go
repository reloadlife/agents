package workspaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortSessionID(t *testing.T) {
	cases := map[string]string{
		"s_01ARZ3NDEKTSV4RRFFQ69G5FAV": "01arz3ndek",
		"01ARZ3NDEK":                   "01arz3ndek",
		"s_AB":                         "ab",
		"":                             "unknown",
	}
	for in, want := range cases {
		if got := ShortSessionID(in); got != want {
			t.Fatalf("ShortSessionID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSanitizeBranch(t *testing.T) {
	ok, err := SanitizeBranch("agents/feature-1")
	if err != nil || ok != "agents/feature-1" {
		t.Fatalf("got %q %v", ok, err)
	}
	if _, err := SanitizeBranch("../evil"); err == nil {
		t.Fatal("expected error for ..")
	}
	if _, err := SanitizeBranch("has spaces"); err == nil {
		t.Fatal("expected error for spaces")
	}
	if _, err := SanitizeBranch(""); err == nil {
		t.Fatal("expected error for empty")
	}
}

func TestCreateAndRemoveSessionWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ws := t.TempDir()
	repo := filepath.Join(ws, "myapp")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v in %s: %v (%s)", args, dir, err, string(out))
		}
	}
	run(repo, "git", "init")
	run(repo, "git", "config", "user.email", "t@t")
	run(repo, "git", "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(repo, "git", "add", "README")
	run(repo, "git", "commit", "-m", "init")

	if !IsGitWorkTree(repo) {
		t.Fatal("expected git work tree")
	}
	root, err := RepoRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if root != repo {
		// macOS / symlink temp dirs may differ; compare cleaned bases
		if filepath.Base(root) != "myapp" {
			t.Fatalf("RepoRoot=%s", root)
		}
	}

	sessionID := "s_01ARZ3NDEKTSV4RRFFQ69G5FAV"
	res, err := CreateSessionWorktree(ws, "myapp", repo, sessionID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRel := "myapp/.agents/worktrees/01arz3ndek"
	if res.Path != wantRel {
		t.Fatalf("path=%q want %q", res.Path, wantRel)
	}
	if res.Branch != "agents/01arz3ndek" {
		t.Fatalf("branch=%q", res.Branch)
	}
	if res.BaseCwd != "myapp" {
		t.Fatalf("base_cwd=%q", res.BaseCwd)
	}
	if st, err := os.Stat(res.Abs); err != nil || !st.IsDir() {
		t.Fatalf("worktree missing: %v", err)
	}
	// Linked worktree: .git is a file
	gitMeta := filepath.Join(res.Abs, ".git")
	st, err := os.Lstat(gitMeta)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDir() {
		t.Fatal("expected linked worktree (.git file)")
	}
	// README should be present in the worktree
	if _, err := os.Stat(filepath.Join(res.Abs, "README")); err != nil {
		t.Fatal(err)
	}
	// Branch should be checked out
	cmd := exec.Command("git", "-C", res.Abs, "branch", "--show-current")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "agents/01arz3ndek" {
		t.Fatalf("current branch=%q", string(out))
	}

	// Custom branch name
	res2, err := CreateSessionWorktree(ws, "myapp", repo, "s_ZZZZZZZZZZABCDEF", "feature/iso-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Branch != "feature/iso-1" {
		t.Fatalf("branch=%q", res2.Branch)
	}
	defer func() { _ = RemoveWorktree(repo, res2.Abs) }()

	// Safety: refuse removing primary repo
	if err := RemoveWorktree(repo, repo); err == nil {
		t.Fatal("expected refuse primary repo")
	}

	if err := RemoveWorktree(repo, res.Abs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Abs); !os.IsNotExist(err) {
		t.Fatalf("worktree still present after remove: %v", err)
	}

	// Non-git cwd
	plain := filepath.Join(ws, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsGitWorkTree(plain) {
		t.Fatal("plain dir should not be git")
	}
	if _, err := CreateSessionWorktree(ws, "plain", plain, "s_NOPE", "", nil); err == nil {
		t.Fatal("expected error for non-git cwd")
	}
}
