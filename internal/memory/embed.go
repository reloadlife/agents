package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Embedder turns text into dense vectors (OpenAI-compatible HTTP API).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
}

// HTTPEmbedder calls an OpenAI-compatible embeddings endpoint.
//
// EmbedURL may be:
//   - base: https://api.openai.com/v1  → POST {base}/embeddings
//   - full: https://host/v1/embeddings → POST as-is
type HTTPEmbedder struct {
	EmbedURL string
	ModelName string
	APIKey   string
	Client   *http.Client
}

func (h *HTTPEmbedder) Model() string { return h.ModelName }

func (h *HTTPEmbedder) endpoint() string {
	u := strings.TrimRight(strings.TrimSpace(h.EmbedURL), "/")
	if strings.HasSuffix(u, "/embeddings") {
		return u
	}
	return u + "/embeddings"
}

func (h *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	// truncate very long inputs (provider limits vary)
	in := make([]string, len(texts))
	for i, t := range texts {
		if len(t) > 24_000 {
			t = t[:24_000]
		}
		in[i] = t
	}
	body := map[string]any{
		"model": h.ModelName,
		"input": in,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}
	cli := h.Client
	if cli == nil {
		cli = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("embed HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("embed decode: %w", err)
	}
	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("embed: empty data")
	}
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(out) {
			continue
		}
		v := make([]float32, len(d.Embedding))
		for i, f := range d.Embedding {
			v[i] = float32(f)
		}
		out[d.Index] = v
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embed: missing vector for index %d", i)
		}
	}
	return out, nil
}

// NewHTTPEmbedder returns nil embedder if url/model empty.
func NewHTTPEmbedder(url, model, apiKey string) Embedder {
	url = strings.TrimSpace(url)
	model = strings.TrimSpace(model)
	if url == "" || model == "" {
		return nil
	}
	return &HTTPEmbedder{
		EmbedURL:  url,
		ModelName: model,
		APIKey:    apiKey,
		Client:    &http.Client{Timeout: 60 * time.Second},
	}
}
