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
	cc := rr.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") && !strings.Contains(cc, "no-store") {
		t.Fatalf("index must not be cacheable, Cache-Control=%q", cc)
	}
	if !strings.Contains(body, `name="agents-build"`) {
		t.Fatalf("expected agents-build meta in index HTML")
	}
	if rr.Header().Get("X-Agents-Build") == "" {
		t.Fatalf("expected X-Agents-Build header")
	}
}

func TestHandlerIndexHTMLNoCache(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	cc := rr.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-store") {
		t.Fatalf("index.html Cache-Control want no-store, got %q", cc)
	}
}

func TestHandlerAssetsImmutableCache(t *testing.T) {
	// Discover a real hashed asset from the embedded index.
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body := rr.Body.String()
	// e.g. src="/assets/index-XXXX.js"
	const needle = `src="/assets/`
	start := strings.Index(body, needle)
	if start < 0 {
		// stylesheet only?
		alt := `href="/assets/`
		start = strings.Index(body, alt)
		if start < 0 {
			t.Skip("no /assets/ reference in index (dev placeholder?)")
		}
		start += len(`href="`)
	} else {
		start += len(`src="`)
	}
	rest := body[start:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		t.Fatalf("could not parse asset path from %q", rest[:min(60, len(rest))])
	}
	assetPath := rest[:end]
	if !strings.HasPrefix(assetPath, "/assets/") {
		t.Fatalf("unexpected asset path %q", assetPath)
	}

	ar := httptest.NewRecorder()
	h.ServeHTTP(ar, httptest.NewRequest(http.MethodGet, assetPath, nil))
	if ar.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d body=%q", assetPath, ar.Code, ar.Body.String()[:min(120, ar.Body.Len())])
	}
	cc := ar.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") && !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("hashed asset should be long-cached, Cache-Control=%q", cc)
	}
}

func TestBuildIDStable(t *testing.T) {
	a := BuildID()
	b := BuildID()
	if a == "" || a == "unknown" {
		t.Fatalf("unexpected build id %q", a)
	}
	if a != b {
		t.Fatalf("BuildID not stable: %q vs %q", a, b)
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
