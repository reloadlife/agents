package tui

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	in := "\x1b[32mhello\x1b[0m world"
	got := stripANSI(in)
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestTailPreview(t *testing.T) {
	raw := ""
	for i := 0; i < 20; i++ {
		raw += "\x1b[31mline " + string(rune('a'+i%26)) + "\x1b[0m\n"
	}
	got := tailPreview(raw, 12)
	lines := strings.Split(got, "\n")
	if len(lines) > 12 {
		t.Fatalf("expected <=12 lines, got %d:\n%s", len(lines), got)
	}
	if got == "" {
		t.Fatal("empty preview")
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ANSI not stripped: %q", got)
	}
}

func TestSessionItemFilterValue(t *testing.T) {
	it := sessionItem{
		id: "s_01abc", name: "fix", agent: "claude",
		cwd: "/home/x/proj", branch: "main", state: "running", worktree: true,
	}
	fv := it.FilterValue()
	for _, want := range []string{"s_01abc", "fix", "claude", "proj", "main", "running", "worktree"} {
		if !strings.Contains(fv, want) {
			t.Fatalf("FilterValue missing %q in %q", want, fv)
		}
	}
}
