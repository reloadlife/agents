package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPEmbedder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		}
		data := make([]item, len(req.Input))
		for i := range req.Input {
			data[i] = item{Index: i, Embedding: []float64{0.1, 0.2, float64(i)}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL+"/v1", "test-model", "key")
	if e == nil {
		t.Fatal("nil embedder")
	}
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("vecs: %+v", vecs)
	}
}

func TestCosineAndPack(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if cosineSim(a, b) < 0.99 {
		t.Fatalf("same vectors should be ~1")
	}
	c := []float32{0, 1, 0}
	if cosineSim(a, c) > 0.01 {
		t.Fatalf("orthogonal ~0")
	}
	raw := packFloat32(a)
	got := unpackFloat32(raw)
	if len(got) != 3 || got[0] != 1 {
		t.Fatalf("pack/unpack: %v", got)
	}
}

func TestSearchVector(t *testing.T) {
	// stub embedder: hash-ish fixed dims
	e := &stubEmbed{}
	s, err := OpenWith(t.TempDir(), OpenOptions{Embedder: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Upsert(Doc{Workspace: ".", Path: "a.md", Text: "alpha document about cats", Source: "doc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(Doc{Workspace: ".", Path: "b.md", Text: "beta document about rockets", Source: "doc"}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchMode(".", "cats", 5, SearchVector)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Mode != "vector" {
		t.Fatalf("mode %s", hits[0].Mode)
	}
}

type stubEmbed struct{}

func (s *stubEmbed) Model() string { return "stub" }
func (s *stubEmbed) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		// crude bag: dim0 = has "cat", dim1 = has "rocket"
		v := []float32{0, 0, 0.1}
		if containsFold(t, "cat") {
			v[0] = 1
		}
		if containsFold(t, "rocket") {
			v[1] = 1
		}
		out[i] = v
	}
	return out, nil
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (stringIndexFold(s, sub) >= 0)))
}

func stringIndexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	for i := 0; i+lsub <= ls; i++ {
		match := true
		for j := 0; j < lsub; j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
