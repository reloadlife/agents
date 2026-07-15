package recording

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStripANSI(t *testing.T) {
	in := "\x1b[32mok\x1b[0m"
	if StripANSI(in) != "ok" {
		t.Fatal(StripANSI(in))
	}
}

func TestArchiveRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("token=ghp_abcdefghijklmnopqrstuvwxyz012345\nnormal line\n")
	meta, err := s.Archive("sess1", "claude", ".", "n", "manual", data)
	if err != nil || meta == nil {
		t.Fatalf("archive: meta=%v err=%v", meta, err)
	}
	_, pane, err := s.Get(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	text := string(pane)
	if strings.Contains(text, "ghp_") {
		t.Fatalf("secret not redacted: %q", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("expected REDACTED marker: %q", text)
	}
	if !strings.Contains(text, "normal line") {
		t.Fatalf("lost content: %q", text)
	}
}

func TestRetentionMaxPerSession(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetRetention(Retention{MaxPerSession: 2})
	for i := 0; i < 5; i++ {
		_, err := s.Archive("sessA", "a", ".", "", "manual", []byte("pane content "+string(rune('a'+i))+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct CreatedAt
	}
	list, err := s.List("sessA", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 retained, got %d", len(list))
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Archive("s", "a", ".", "", "manual", []byte("hello searchable unique-xyz\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(meta.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(meta.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestSearchWithSnippets(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Archive("s", "claude", "repo", "name", "manual", []byte("before unique-token-abc after more\n"))
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchWithSnippets("unique-token-abc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits=%d", len(hits))
	}
	if !strings.Contains(hits[0].Snippet, "unique-token-abc") {
		t.Fatalf("snippet=%q", hits[0].Snippet)
	}
	// ensure files exist on disk
	ents, _ := os.ReadDir(filepath.Join(dir, "s"))
	if len(ents) < 2 {
		t.Fatalf("disk ents=%d", len(ents))
	}
}
