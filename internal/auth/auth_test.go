package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareBearer(t *testing.T) {
	h := Middleware("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// public healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("healthz: %d", rr.Code)
	}

	// public static UI shell (no token)
	for _, path := range []string{"/", "/index.html", "/assets/index.js", "/favicon.ico"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("public %s: want 200, got %d", path, rr.Code)
		}
	}

	// missing token
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}

	// good token
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d", rr.Code)
	}

	// query token (websocket-friendly)
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/x/pty?token=secret", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("query token: want 200, got %d", rr.Code)
	}
}

func TestMiddlewareOptsMultiToken(t *testing.T) {
	tokens := map[string]string{
		"default": "primary-token-value",
		"ops":     "ops-token-value-xx",
		"guest":   "guest-token-value",
	}
	var gotActor string
	h := MiddlewareOpts(Options{
		Tokens:        tokens,
		RequireBearer: true,
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = ActorFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// each token label authenticates
	for label, tok := range tokens {
		gotActor = ""
		req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("label %s: want 200, got %d", label, rr.Code)
		}
		if gotActor != label {
			t.Fatalf("label %s: actor=%q", label, gotActor)
		}
	}

	// wrong token
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer no-such-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("bad token: want 401, got %d", rr.Code)
	}

	// empty token values in map are ignored
	h2 := MiddlewareOpts(Options{
		Tokens:        map[string]string{"empty": "", "ok": "good-token"},
		RequireBearer: true,
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rr = httptest.NewRecorder()
	h2.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("non-empty token in map: want 200, got %d", rr.Code)
	}
}

func TestMiddlewareOptsTrustedHeaderRequireBearer(t *testing.T) {
	var gotActor string
	h := MiddlewareOpts(Options{
		Tokens:        map[string]string{"default": "secret-token-xx"},
		TrustedHeader: "Tailscale-User-Login",
		RequireBearer: true,
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = ActorFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// header alone is not enough when require_bearer is true
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("header-only with require_bearer: want 401, got %d", rr.Code)
	}

	// bearer + trusted header → actor is label/header
	gotActor = ""
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token-xx")
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("bearer+header: want 200, got %d", rr.Code)
	}
	if gotActor != "default/alice@example.com" {
		t.Fatalf("actor: got %q", gotActor)
	}

	// bearer without header → label only
	gotActor = ""
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token-xx")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("bearer only: want 200, got %d", rr.Code)
	}
	if gotActor != "default" {
		t.Fatalf("actor without header: got %q", gotActor)
	}
}

func TestMiddlewareOptsRequireBearerFalse(t *testing.T) {
	var gotActor string
	h := MiddlewareOpts(Options{
		Tokens:        map[string]string{"default": "secret-token-xx"},
		TrustedHeader: "Cf-Access-Authenticated-User-Email",
		RequireBearer: false,
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = ActorFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// trusted header alone authenticates
	gotActor = ""
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Cf-Access-Authenticated-User-Email", "bob@example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("header-only auth: want 200, got %d", rr.Code)
	}
	if gotActor != "proxy:bob@example.com" {
		t.Fatalf("proxy actor: got %q", gotActor)
	}

	// no bearer and no header → 401
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("nothing: want 401, got %d", rr.Code)
	}

	// bearer still works
	gotActor = ""
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token-xx")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("bearer with require_bearer false: want 200, got %d", rr.Code)
	}
	if gotActor != "default" {
		t.Fatalf("bearer actor: got %q", gotActor)
	}

	// query token also works
	gotActor = ""
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/x/pty?token=secret-token-xx", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("query token: want 200, got %d", rr.Code)
	}
}

func TestIsPublicPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/healthz", true},
		{"/", true},
		{"/assets/app.js", true},
		{"/projects", true},
		{"/v1", false},
		{"/v1/status", false},
		{"/v1/sessions/x/pty", false},
	}
	for _, tc := range cases {
		if got := IsPublicPath(tc.path); got != tc.want {
			t.Fatalf("IsPublicPath(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
}
