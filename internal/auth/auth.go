package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Middleware enforces Authorization: Bearer <token>.
// WebSocket clients may also pass ?token= for environments that cannot set headers.
func Middleware(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// healthz stays public for tunnel probes
		if r.URL.Path == "/healthz" {
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
