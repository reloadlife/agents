// Package webui serves the embedded browser UI for agentsd.
package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

// dist holds the built SPA (vite → dist/). Rebuild with `make web`.
//
//go:embed all:dist
var distFS embed.FS

var (
	buildOnce sync.Once
	buildID   string
)

// BuildID returns a short content fingerprint of the embedded SPA index.
// Changes whenever the web UI is rebuilt into the binary.
func BuildID() string {
	buildOnce.Do(func() {
		b, err := distFS.ReadFile("dist/index.html")
		if err != nil {
			buildID = "unknown"
			return
		}
		sum := sha256.Sum256(b)
		buildID = hex.EncodeToString(sum[:8])
	})
	return buildID
}

// Handler returns an http.Handler that serves the embedded SPA.
// Unknown non-file paths fall back to index.html for client-side routing.
//
// Caching policy (so a normal browser refresh picks up updates after agentsd upgrade):
//   - index.html / SPA shell: never cache (no-store)
//   - /assets/* (content-hashed by Vite): long-lived immutable cache
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
		path = strings.TrimPrefix(path, "./")

		// SPA shell must never be cached — always revalidate on simple refresh.
		if path == "" || path == "index.html" {
			serveIndex(w, sub)
			return
		}

		// If the path is a real file under dist/, serve it with asset-appropriate caching.
		if f, err := sub.Open(path); err == nil {
			stat, stErr := f.Stat()
			_ = f.Close()
			// Directories under dist (if any) are not SPA assets — fall through.
			if stErr == nil && !stat.IsDir() {
				if strings.HasPrefix(path, "assets/") {
					// Vite content hashes in filenames → safe to pin forever.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
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
	// Inject a build marker so the client can detect UI updates without hard-refresh.
	id := BuildID()
	html := string(b)
	if !strings.Contains(html, `name="agents-build"`) {
		marker := `<meta name="agents-build" content="` + id + `"/>`
		if i := strings.Index(html, "<head>"); i >= 0 {
			i += len("<head>")
			html = html[:i] + "\n  " + marker + html[i:]
		} else {
			html = marker + html
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Strong no-cache so F5 / location.reload() always re-fetches the shell
	// (and therefore the new hashed /assets/* URLs).
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Agents-Build", id)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
