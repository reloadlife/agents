package pathallow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAllow(t *testing.T) {
	root := t.TempDir()
	must := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	must("DollarChande")
	must("reloadlife/mamad.dev")
	must("mamaru/madcore")

	patterns := []string{"DollarChande", "reloadlife/*", "mamaru/*"}

	abs, err := Resolve(root, "DollarChande", patterns)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(abs) != "DollarChande" {
		t.Fatalf("got %s", abs)
	}

	if _, err := Resolve(root, "reloadlife/mamad.dev", patterns); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "../etc", patterns); err == nil {
		t.Fatal("expected .. reject")
	}
	if _, err := Resolve(root, "secret-stuff", patterns); err == nil {
		t.Fatal("expected allowlist reject")
	}
	must("DollarChande/src")
	if _, err := Resolve(root, "DollarChande/src", []string{"DollarChande"}); err != nil {
		t.Fatal(err)
	}
	// "." allows everything under workspace
	if _, err := Resolve(root, "secret-stuff", []string{"."}); err != nil {
		t.Fatal(err)
	}
}
