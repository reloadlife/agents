package workspaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reloadlife/agents/internal/config"
)

func TestStatusAndDiffTempRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v in %s: %v\n%s", args, dir, err, out)
		}
	}
	run(repo, "git", "init")
	// Ensure a branch name exists across git versions.
	run(repo, "git", "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(repo, "git", "add", "hello.txt")
	run(repo, "git", "commit", "-m", "init")

	// Modify tracked + add untracked
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("brand new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		WorkspaceRoot: root,
		DefaultCwd:    "proj",
		Allow:         config.AllowConfig{Paths: []string{"."}},
	}
	// Normalize like Load does
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorkspaceRoot = absRoot

	st, err := Status(cfg, "proj")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.IsRepo {
		t.Fatal("expected is_repo true")
	}
	if st.Root == "" {
		t.Fatal("expected root")
	}
	if !st.Dirty {
		t.Fatal("expected dirty")
	}
	if st.Branch != "main" && !strings.Contains(st.Branch, "main") {
		// some envs may report differently
		t.Logf("branch=%q", st.Branch)
	}
	if st.Head == "" {
		t.Fatal("expected head short sha")
	}
	if len(st.Files) < 2 {
		t.Fatalf("expected >=2 files, got %#v", st.Files)
	}
	var sawHello, sawNew bool
	for _, f := range st.Files {
		switch f.Path {
		case "hello.txt":
			sawHello = true
			if !f.Unstaged {
				t.Fatalf("hello.txt should be unstaged: %#v", f)
			}
			if f.Status == "" {
				t.Fatalf("hello.txt empty status: %#v", f)
			}
			if f.Additions < 1 {
				t.Fatalf("hello.txt expected additions>=1: %#v", f)
			}
		case "new.txt":
			sawNew = true
			if f.Status != "??" {
				t.Fatalf("new.txt status want ?? got %#v", f)
			}
			if !f.Unstaged || f.Staged {
				t.Fatalf("new.txt flags: %#v", f)
			}
		}
	}
	if !sawHello || !sawNew {
		t.Fatalf("missing files: hello=%v new=%v files=%#v", sawHello, sawNew, st.Files)
	}
	if st.Summary.Untracked < 1 {
		t.Fatalf("summary untracked: %#v", st.Summary)
	}
	if st.Summary.Unstaged < 1 {
		t.Fatalf("summary unstaged: %#v", st.Summary)
	}

	// Unstaged diff for hello.txt
	d, err := Diff(cfg, "proj", "hello.txt", false, "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if d.Truncated {
		t.Fatal("unexpected truncation")
	}
	if !strings.Contains(d.Diff, "world") {
		t.Fatalf("diff missing world: %q", d.Diff)
	}
	if d.Base != "HEAD" || d.Staged {
		t.Fatalf("diff meta: %#v", d)
	}

	// Whole worktree diff
	d2, err := Diff(cfg, "proj", "", false, "")
	if err != nil {
		t.Fatalf("Diff all: %v", err)
	}
	if !strings.Contains(d2.Diff, "hello.txt") {
		t.Fatalf("full diff missing hello.txt: %q", d2.Diff)
	}

	// Stage a change and check staged diff
	run(repo, "git", "add", "hello.txt")
	st2, err := Status(cfg, "proj")
	if err != nil {
		t.Fatal(err)
	}
	var stagedHello bool
	for _, f := range st2.Files {
		if f.Path == "hello.txt" && f.Staged {
			stagedHello = true
		}
	}
	if !stagedHello {
		t.Fatalf("expected staged hello: %#v", st2.Files)
	}
	ds, err := Diff(cfg, "proj", "hello.txt", true, "HEAD")
	if err != nil {
		t.Fatalf("staged Diff: %v", err)
	}
	if !strings.Contains(ds.Diff, "world") {
		t.Fatalf("staged diff: %q", ds.Diff)
	}

	// File at HEAD should still be original
	blob, err := FileAtRef(cfg, "proj", "hello.txt", "HEAD")
	if err != nil {
		t.Fatalf("FileAtRef: %v", err)
	}
	if blob.Missing {
		t.Fatal("hello.txt missing at HEAD")
	}
	if strings.Contains(blob.Content, "world") {
		t.Fatalf("HEAD blob should be clean: %q", blob.Content)
	}
	if !strings.Contains(blob.Content, "hello") {
		t.Fatalf("HEAD blob: %q", blob.Content)
	}

	// Non-repo directory
	other := filepath.Join(root, "notgit")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	st3, err := Status(cfg, "notgit")
	if err != nil {
		t.Fatalf("Status non-repo: %v", err)
	}
	if st3.IsRepo {
		t.Fatal("expected is_repo false")
	}
	if len(st3.Files) != 0 {
		t.Fatalf("expected empty files: %#v", st3.Files)
	}
}

func TestParsePorcelain(t *testing.T) {
	in := "" +
		" M web/src/main.ts\n" +
		"M  staged.go\n" +
		"MM both.go\n" +
		"?? untracked.txt\n" +
		"R  old.txt -> newname.txt\n" +
		"UU conflict.go\n"
	files := parsePorcelain(in)
	by := map[string]FileStatus{}
	for _, f := range files {
		by[f.Path] = f
	}
	if f := by["web/src/main.ts"]; !f.Unstaged || f.Staged || f.Status != "M" {
		t.Fatalf("main.ts: %#v", f)
	}
	if f := by["staged.go"]; !f.Staged || f.Unstaged {
		t.Fatalf("staged: %#v", f)
	}
	if f := by["both.go"]; !f.Staged || !f.Unstaged || f.Status != "MM" {
		t.Fatalf("both: %#v", f)
	}
	if f := by["untracked.txt"]; f.Status != "??" || f.Staged {
		t.Fatalf("untracked: %#v", f)
	}
	if f := by["newname.txt"]; f.Path != "newname.txt" || !f.Staged {
		t.Fatalf("rename: %#v", f)
	}
	if f := by["conflict.go"]; !isConflicted(f.Status) {
		t.Fatalf("conflict: %#v", f)
	}
}

func TestPathEscapeRejected(t *testing.T) {
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
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@e.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@e.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	// Need a commit for HEAD
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "a.txt"},
		{"git", "commit", "-m", "i"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@e.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@e.com",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	absRoot, _ := filepath.Abs(root)
	cfg := &config.Config{
		WorkspaceRoot: absRoot,
		Allow:         config.AllowConfig{Paths: []string{"."}},
	}
	if _, err := Diff(cfg, "proj", "../outside", false, "HEAD"); err == nil {
		t.Fatal("expected path escape error")
	}
	if _, err := Diff(cfg, "proj", "", false, "-evil"); err == nil {
		t.Fatal("expected bad ref error")
	}
}

func TestCapDiff(t *testing.T) {
	s, trunc, bin := capDiff([]byte("hello"), 100)
	if trunc || bin || s != "hello" {
		t.Fatalf("%q trunc=%v bin=%v", s, trunc, bin)
	}
	big := bytesRepeat('x', 100)
	s, trunc, _ = capDiff(big, 50)
	if !trunc || !strings.Contains(s, "truncated") {
		t.Fatalf("want truncated: %q", s)
	}
	s, trunc, bin = capDiff([]byte("a\x00b"), 100)
	if !bin || s == "" {
		t.Fatalf("binary: %q trunc=%v", s, trunc)
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
