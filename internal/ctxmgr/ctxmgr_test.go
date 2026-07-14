package ctxmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reloadlife/agents/internal/memory"
)

func TestEnsureAndPack(t *testing.T) {
	root := t.TempDir()
	// minimal project
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo\n\nHello agents.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("## Rules\nUse the map first.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	memDir := t.TempDir()
	store, err := memory.Open(memDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := New(store)
	st := m.ReadStatus(root, "demo")
	if st.Ready {
		t.Fatal("expected not ready before ensure")
	}

	res, err := m.Ensure(root, "demo", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.MapGenerated {
		t.Fatal("expected map generated")
	}
	if res.MemoryIndexed == 0 {
		t.Fatal("expected memory index")
	}
	if !res.ContextWrote {
		t.Fatal("expected CONTEXT.md write")
	}
	if !res.Status.Ready {
		t.Fatalf("expected ready, hints=%v", res.Status.Hints)
	}

	mapPath := filepath.Join(root, ".agents", "PROJECT_MAP.md")
	if _, err := os.Stat(mapPath); err != nil {
		t.Fatal(err)
	}
	ctxPath := filepath.Join(root, ".agents", ContextFile)
	b, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Project map") {
		t.Fatalf("CONTEXT.md missing map section: %s", b[:min(200, len(b))])
	}
	instr := filepath.Join(root, ".agents", InstructionsFile)
	if _, err := os.Stat(instr); err != nil {
		t.Fatal(err)
	}

	seed := ComposeSeed("demo", "fix the auth bug", 2000)
	if !strings.Contains(seed, "PROJECT_MAP") || !strings.Contains(seed, "fix the auth bug") {
		t.Fatalf("bad seed: %s", seed)
	}

	doc, err := m.Note("demo", "decision", "We use JWT for API auth.")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source != "note" {
		t.Fatalf("source=%s", doc.Source)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
