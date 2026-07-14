package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Middleware enforces Authorization: Bearer <token> on API routes.
// WebSocket clients may also pass ?token= for environments that cannot set headers.
// Public (no token): /healthz and non-/v1 paths (embedded web UI static shell).
func Middleware(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearer(r.Header.Get("Authorization"))
		if got == "" {
			got = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if len(got) == 0 || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsPublicPath reports whether path may be served without a bearer token.
// API under /v1 is always authenticated; health probes and the static web UI are public.
func IsPublicPath(path string) bool {
	if path == "/healthz" {
		return true
	}
	// Control plane API — always require auth (including bare /v1).
	if path == "/v1" || strings.HasPrefix(path, "/v1/") {
		return false
	}
	// Static web UI (and any non-API path mounted by agentsd).
	return true
}

func bearer(h string) string {
	const p = "Bearer "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
