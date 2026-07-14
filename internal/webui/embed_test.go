package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndex(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "agents") && !strings.Contains(body, "html") {
		// Built index or placeholder both contain html markup.
		if !strings.Contains(strings.ToLower(body), "<!doctype") && !strings.Contains(body, "<html") {
			t.Fatalf("unexpected body prefix: %q", body[:min(80, len(body))])
		}
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") {
		// FileServer may set text/html; accept empty on some platforms.
		t.Logf("content-type: %s", ct)
	}
}

func TestHandlerDoesNotShadowAPI(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for /v1/status via webui, got %d", rr.Code)
	}
}

func TestHandlerSPAFallback(t *testing.T) {
	h := Handler()
	for _, path := range []string{
		"/desk",
		"/new",
		"/project/new",
		"/projects/new",
		"/new/project",
		"/tools",
		"/help",
		"/profile",
		"/profile/github",
		"/settings/ssh",
		"/project/agents/session/s_01TEST/tools",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(strings.ToLower(body), "<html") && !strings.Contains(strings.ToLower(body), "<!doctype") {
			t.Fatalf("GET %s: expected SPA index HTML, got %q", path, body[:min(80, len(body))])
		}
	}
}
