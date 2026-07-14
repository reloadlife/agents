package workspaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reloadlife/agents/internal/config"
)

func TestCommitTempRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=reloadlife",
			"GIT_AUTHOR_EMAIL=me@mamad.dev",
			"GIT_COMMITTER_NAME=reloadlife",
			"GIT_COMMITTER_EMAIL=me@mamad.dev",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.name", "reloadlife")
	run("git", "config", "user.email", "me@mamad.dev")
	run("git", "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"**"}},
	}

	res, err := Commit(cfg, CommitRequest{
		Cwd:     "proj",
		Message: "test: initial commit",
		All:     true,
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected ok: %+v", res)
	}
	if res.Cwd != "proj" {
		t.Fatalf("cwd=%q want proj", res.Cwd)
	}
	if res.Message != "test: initial commit" {
		t.Fatalf("message=%q", res.Message)
	}
	if len(res.Commit) < 7 {
		t.Fatalf("commit hash too short: %q", res.Commit)
	}
	if res.Branch != "main" {
		t.Fatalf("branch=%q want main", res.Branch)
	}

	cmd := exec.Command("git", "-C", repo, "log", "-1", "--pretty=%s")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "test: initial commit" {
		t.Fatalf("log subject=%q", got)
	}

	// Second commit via explicit paths.
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := Commit(cfg, CommitRequest{
		Cwd:     "proj",
		Message: "test: update hello",
		Paths:   []string{"hello.txt"},
	})
	if err != nil {
		t.Fatalf("Commit paths: %v", err)
	}
	if res2.Commit == res.Commit {
		t.Fatalf("expected new commit hash, got same %s", res2.Commit)
	}

	// Nothing to commit should fail.
	if _, err := Commit(cfg, CommitRequest{Cwd: "proj", Message: "noop", All: true}); err == nil {
		t.Fatal("expected nothing to commit error")
	}
}

func TestCommitRequiresMessageAndRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nongit"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"**"}},
	}

	if _, err := Commit(cfg, CommitRequest{Cwd: "nongit", Message: ""}); err == nil {
		t.Fatal("expected empty message error")
	}
	if _, err := Commit(cfg, CommitRequest{Cwd: "nongit", Message: "x", All: true}); err == nil {
		t.Fatal("expected not a git repo error")
	}
}

func TestParsePRCreateOutput(t *testing.T) {
	url, n := parsePRCreateOutput("https://github.com/o/r/pull/42\n")
	if url != "https://github.com/o/r/pull/42" || n != 42 {
		t.Fatalf("%s %d", url, n)
	}
	url, n = parsePRCreateOutput("Warning: something\n  https://github.com/o/r/pull/7  \n")
	if url != "https://github.com/o/r/pull/7" || n != 7 {
		t.Fatalf("%s %d", url, n)
	}
}

func TestCommitRejectsPathEscape(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"**"}},
	}
	if _, err := Commit(cfg, CommitRequest{
		Cwd:     "proj",
		Message: "evil",
		Paths:   []string{"../outside"},
	}); err == nil {
		t.Fatal("expected path escape rejection")
	}
}
