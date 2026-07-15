package memory

import (
	"context"
	"testing"
)

type countingEmbed struct {
	calls int
}

func (c *countingEmbed) Model() string { return "count" }
func (c *countingEmbed) Embed(_ context.Context, texts []string) ([][]float32, error) {
	c.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i + 1), 0.5, 0.25}
	}
	return out, nil
}

func TestReembedMissing(t *testing.T) {
	// First open without embedder so docs land without vectors.
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Upsert(Doc{Workspace: "ws", Path: "a.md", Text: "alpha document one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(Doc{Workspace: "ws", Path: "b.md", Text: "beta document two"}); err != nil {
		t.Fatal(err)
	}
	with, _ := s.VectorStats("ws")
	if with != 0 {
		t.Fatalf("expected 0 embedded, got %d", with)
	}

	emb := &countingEmbed{}
	s.SetEmbedder(emb)
	rr, err := s.ReembedMissing("ws")
	if err != nil {
		t.Fatal(err)
	}
	if rr.Embedded != 2 || rr.Failed != 0 {
		t.Fatalf("reembed result: %+v", rr)
	}
	with, _ = s.VectorStats("ws")
	if with != 2 {
		t.Fatalf("after reembed embedded=%d", with)
	}
	// second pass: all skipped
	rr2, err := s.ReembedMissing("ws")
	if err != nil {
		t.Fatal(err)
	}
	if rr2.Embedded != 0 || rr2.Skipped != 2 {
		t.Fatalf("second pass: %+v", rr2)
	}
}

func TestReembedWithoutEmbedder(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = s.Upsert(Doc{Workspace: "ws", Path: "a.md", Text: "text"})
	_, err = s.ReembedMissing("ws")
	if err == nil {
		t.Fatal("expected error when embedder missing")
	}
}
