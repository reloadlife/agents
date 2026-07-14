// Package webui serves the embedded browser UI for agentsd.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the built SPA (vite → dist/). Rebuild with `make web`.
//
//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA.
// Unknown non-file paths fall back to index.html for client-side routing.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "webui embed broken", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never shadow API or health routes if mounted at root incorrectly.
		if r.URL.Path == "/healthz" || r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// Clean leading "./" etc. but keep as relative embed path.
		path = strings.TrimPrefix(path, "./")

		// If the path is a real file under dist/, serve it.
		if f, err := sub.Open(path); err == nil {
			stat, stErr := f.Stat()
			_ = f.Close()
			// Directories under dist (if any) are not SPA assets — fall through.
			if stErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Asset misses should 404 (not return HTML).
		if strings.HasPrefix(path, "assets/") {
			http.NotFound(w, r)
			return
		}
		// SPA fallback: always serve index.html content (no FileServer rewrite tricks).
		serveIndex(w, sub)
	})
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "index.html missing from webui embed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
