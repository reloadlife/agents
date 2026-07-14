package projmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAndWrite(t *testing.T) {
	root := t.TempDir()
	// fixture layout
	mustWrite(t, filepath.Join(root, "README.md"), "# hello\n")
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\nfunc main() {}\n")
	mustWrite(t, filepath.Join(root, "internal", "pkg", "x.go"), "package pkg\n")
	mustWrite(t, filepath.Join(root, "node_modules", "x", "index.js"), "/* noise */\n")
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0o755)

	m, metaPath, err := GenerateAndWrite(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	if metaPath == "" {
		t.Fatal("empty meta path")
	}
	if m.Root != root {
		t.Fatalf("root: %s", m.Root)
	}
	foundGo := false
	for _, s := range m.Stack {
		if s == "Go" {
			foundGo = true
		}
	}
	if !foundGo {
		t.Fatalf("expected Go in stack: %v", m.Stack)
	}
	// node_modules should not appear in layout paths
	for _, d := range m.Layout {
		if strings.Contains(d.Path, "node_modules") {
			t.Fatalf("node_modules leaked into layout: %s", d.Path)
		}
	}
	md, err := ReadMarkdown(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "Project map") {
		t.Fatalf("markdown missing title: %s", md[:min(80, len(md))])
	}
	if !strings.Contains(md, "cmd/app/main.go") && len(m.Entrypoints) == 0 {
		t.Fatal("expected entrypoint detection")
	}

	st := ReadStatus(root)
	if !st.Exists {
		t.Fatal("status should exist")
	}
	// no git repo with HEAD → not stale from git; ok either way
	_ = st
}

func TestReadStatusMissing(t *testing.T) {
	root := t.TempDir()
	st := ReadStatus(root)
	if st.Exists {
		t.Fatal("should not exist")
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
