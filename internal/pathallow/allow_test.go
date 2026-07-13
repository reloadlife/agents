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
	must("my-app")
	must("team/website")
	must("org/service")

	patterns := []string{"my-app", "team/*", "org/*"}

	abs, err := Resolve(root, "my-app", patterns)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(abs) != "my-app" {
		t.Fatalf("got %s", abs)
	}

	if _, err := Resolve(root, "team/website", patterns); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "../etc", patterns); err == nil {
		t.Fatal("expected .. reject")
	}
	if _, err := Resolve(root, "secret-stuff", patterns); err == nil {
		t.Fatal("expected allowlist reject")
	}
	must("my-app/src")
	if _, err := Resolve(root, "my-app/src", []string{"my-app"}); err != nil {
		t.Fatal(err)
	}
	// "." allows everything under workspace
	if _, err := Resolve(root, "secret-stuff", []string{"."}); err != nil {
		t.Fatal(err)
	}
}
