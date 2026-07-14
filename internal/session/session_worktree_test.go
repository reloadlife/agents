package session

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/workspaces"
)

func TestDeleteRemovesWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	jobs := filepath.Join(tmp, "jobs")
	repo := filepath.Join(ws, "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jobs, 0o700); err != nil {
		t.Fatal(err)
	}

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v (%s)", args, err, out)
		}
	}
	run(repo, "git", "init")
	run(repo, "git", "config", "user.email", "t@t")
	run(repo, "git", "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(repo, "git", "add", "f")
	run(repo, "git", "commit", "-m", "init")

	cfg := &config.Config{
		JobsDir:       jobs,
		WorkspaceRoot: ws,
		DefaultCwd:    ".",
	}
	m, err := NewManager(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	id := "s_01TESTDELWT0"
	wt, err := workspaces.CreateSessionWorktree(ws, "app", repo, id, "agents/test-del", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Abs); err != nil {
		t.Fatal(err)
	}

	// Inject a stopped session that owns the worktree (no tmux required).
	s := &Session{
		ID:           id,
		Agent:        "shell",
		Mode:         ModeTTY,
		Cwd:          wt.Path,
		CwdAbs:       wt.Abs,
		Tmux:         "la-nonexistent-worktree-test",
		State:        StateExited,
		CreatedAt:    time.Now().UTC(),
		Worktree:     true,
		WorktreePath: wt.Path,
		BaseCwd:      "app",
		Branch:       wt.Branch,
	}
	if err := m.save(s); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.byID[s.ID] = s
	m.mu.Unlock()

	if err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Abs); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after Delete: %v", err)
	}
	// Main repo must remain
	if st, err := os.Stat(repo); err != nil || !st.IsDir() {
		t.Fatalf("main repo missing: %v", err)
	}
	if _, err := m.Get(id); err == nil {
		t.Fatal("session should be gone")
	}
}
