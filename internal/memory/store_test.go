package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Upsert(Doc{
		Workspace: ".",
		Path:      "README.md",
		Title:     "README",
		Source:    "doc",
		Text:      "agents is a remote control plane for coding agent CLIs with tmux sessions",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(Doc{
		Workspace: ".",
		Path:      "notes.md",
		Title:     "notes",
		Source:    "note",
		Text:      "vector memory and project maps help models orient",
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(".", "tmux sessions", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits for tmux")
	}

	n, err := s.Stats(".")
	if err != nil || n != 2 {
		t.Fatalf("stats: n=%d err=%v", n, err)
	}
}

func TestIndexWorkspace(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# Demo\n\nThis is the demo project for memory indexing.\n")
	mustWrite(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n\nHow to use the demo control plane.\n")
	mustWrite(t, filepath.Join(root, ".agents", "PROJECT_MAP.md"), "# Project map\n\nDemo layout.\n")

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n, err := s.IndexWorkspace("demo", root, IndexOptions{Clear: true})
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("expected >=2 docs, got %d", n)
	}
	hits, err := s.Search("demo", "control plane", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected search hits")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
