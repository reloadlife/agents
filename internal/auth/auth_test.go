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
