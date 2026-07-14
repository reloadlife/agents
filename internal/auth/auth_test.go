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

func TestIsPublicPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/healthz", true},
		{"/", true},
		{"/assets/app.js", true},
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
