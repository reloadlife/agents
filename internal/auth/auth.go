package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type ctxKey int

const actorKey ctxKey = 1

// ActorFrom returns the authenticated principal label (token label or trusted header).
func ActorFrom(ctx context.Context) string {
	if v, ok := ctx.Value(actorKey).(string); ok {
		return v
	}
	return ""
}

// Options configures multi-token + optional trusted reverse-proxy identity.
type Options struct {
	// Tokens maps label → raw token value.
	Tokens map[string]string
	// TrustedHeader e.g. Tailscale-User-Login or Cf-Access-Authenticated-User-Email
	TrustedHeader string
	// RequireBearer when true, a valid bearer is required (default).
	// When false, trusted header alone may authenticate.
	RequireBearer bool
}

// Middleware enforces Authorization: Bearer <token> on API routes.
// WebSocket clients may also pass ?token= for environments that cannot set headers.
// Public (no token): /healthz and non-/v1 paths (embedded web UI static shell).
func Middleware(token string, next http.Handler) http.Handler {
	return MiddlewareOpts(Options{
		Tokens:        map[string]string{"default": token},
		RequireBearer: true,
	}, next)
}

// MiddlewareOpts supports multi-token maps and trusted identity headers.
func MiddlewareOpts(opt Options, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearer(r.Header.Get("Authorization"))
		if got == "" {
			got = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		label := matchToken(got, opt.Tokens)
		headerID := ""
		if opt.TrustedHeader != "" {
			headerID = strings.TrimSpace(r.Header.Get(opt.TrustedHeader))
		}

		ok := label != ""
		if !ok && !opt.RequireBearer && headerID != "" {
			ok = true
			label = "proxy:" + headerID
		}
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if headerID != "" && !strings.HasPrefix(label, "proxy:") {
			label = label + "/" + headerID
		}
		ctx := context.WithValue(r.Context(), actorKey, label)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func matchToken(got string, tokens map[string]string) string {
	if got == "" || len(tokens) == 0 {
		return ""
	}
	gb := []byte(got)
	for label, want := range tokens {
		if want == "" {
			continue
		}
		wb := []byte(want)
		if len(gb) == len(wb) && subtle.ConstantTimeCompare(gb, wb) == 1 {
			return label
		}
	}
	return ""
}

// IsPublicPath reports whether path may be served without a bearer token.
func IsPublicPath(path string) bool {
	if path == "/healthz" {
		return true
	}
	if path == "/v1" || strings.HasPrefix(path, "/v1/") {
		return false
	}
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
