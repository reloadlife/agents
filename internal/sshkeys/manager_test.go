package sshkeys

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	ok, err := sanitizeName("id_agents")
	if err != nil || ok != "id_agents" {
		t.Fatalf("got %q %v", ok, err)
	}
	if _, err := sanitizeName("../evil"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := sanitizeName("foo/bar"); err == nil {
		t.Fatal("expected error")
	}
	got, err := sanitizeName("id_x.pub")
	if err != nil || got != "id_x" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestGenerateListDelete(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen missing")
	}
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	k, err := m.Generate(GenerateRequest{Name: "id_test_agents", Comment: "test@agents"})
	if err != nil {
		t.Fatal(err)
	}
	if !k.HasPrivate || !k.HasPublic {
		t.Fatalf("missing files: %+v", k)
	}
	if k.PublicKey == "" || k.Fingerprint == "" {
		t.Fatalf("missing pub/fp: %+v", k)
	}
	// private must not appear in JSON-ish fields (we never set it)
	if _, err := os.Stat(filepath.Join(dir, "id_test_agents")); err != nil {
		t.Fatal(err)
	}
	list, err := m.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}
	if err := m.Delete("id_test_agents"); err != nil {
		t.Fatal(err)
	}
	list, err = m.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("after delete: %v %#v", err, list)
	}
}

func TestRefuseProtectedDelete(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("config"); err == nil {
		t.Fatal("expected refuse")
	}
}
