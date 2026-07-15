package workspaces

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reloadlife/agents/internal/config"
)

func TestCreateWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"."}},
	}
	e, err := Create(cfg, CreateRequest{Name: "my-app"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Path != "my-app" {
		t.Fatalf("path=%q", e.Path)
	}
	st, err := os.Stat(filepath.Join(root, "my-app"))
	if err != nil || !st.IsDir() {
		t.Fatalf("dir missing: %v", err)
	}
	// idempotent
	if _, err := Create(cfg, CreateRequest{Name: "my-app"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateNestedAndReject(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"."}},
	}
	if _, err := Create(cfg, CreateRequest{Name: "clients/acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(cfg, CreateRequest{Name: "../escape"}); err == nil {
		t.Fatal("expected escape reject")
	}
	if _, err := Create(cfg, CreateRequest{Name: ".hidden"}); err == nil {
		t.Fatal("expected hidden reject")
	}
	if _, err := Create(cfg, CreateRequest{Name: ""}); err == nil {
		t.Fatal("expected empty reject")
	}
}
