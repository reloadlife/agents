package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitBranchRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")

	got := gitBranch(dir)
	if got != "main" {
		t.Fatalf("gitBranch = %q, want main", got)
	}

	run("checkout", "-b", "agents/s01demo")
	got = gitBranch(dir)
	if got != "agents/s01demo" {
		t.Fatalf("gitBranch after checkout = %q, want agents/s01demo", got)
	}
}

func TestGitBranchNonRepo(t *testing.T) {
	dir := t.TempDir()
	if got := gitBranch(dir); got != "" {
		t.Fatalf("non-repo gitBranch = %q, want empty", got)
	}
	if got := gitBranch(""); got != "" {
		t.Fatalf("empty cwd gitBranch = %q, want empty", got)
	}
	if got := gitBranch("/no/such/path/agents-test"); got != "" {
		t.Fatalf("missing path gitBranch = %q, want empty", got)
	}
}

func TestEnrichGitBranchCacheAndFallback(t *testing.T) {
	dir := t.TempDir()
	// non-repo: should fall back to Session.Branch
	s := &Session{CwdAbs: dir, Branch: "agents/from-meta"}
	cache := map[string]string{}
	enrichGitBranch(s, cache)
	if s.GitBranch != "agents/from-meta" {
		t.Fatalf("fallback GitBranch = %q, want agents/from-meta", s.GitBranch)
	}
	// cache should store empty live result
	if v, ok := cache[dir]; !ok || v != "" {
		t.Fatalf("cache[%s] = %q ok=%v, want empty string", dir, v, ok)
	}
	// second session same cwd hits cache, still gets its own meta fallback
	s2 := &Session{CwdAbs: dir, Branch: "other"}
	enrichGitBranch(s2, cache)
	if s2.GitBranch != "other" {
		t.Fatalf("cached fallback GitBranch = %q, want other", s2.GitBranch)
	}
}
