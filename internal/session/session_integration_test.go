//go:build integration

package session_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reloadlife/agents/internal/api"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/job"
	"github.com/reloadlife/agents/internal/session"
)

// TestIntegration_MockSession starts the API, creates a mock agent session in tmux,
// verifies list/get/kill. Requires tmux on PATH.
func TestIntegration_MockSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	tmp := t.TempDir()
	jobs := filepath.Join(tmp, "jobs")
	ws := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ws, "README"), []byte("hi"), 0o644)

	// mock agent: long-running so session stays "running"
	mockBin := filepath.Join(tmp, "mock-agent")
	script := "#!/bin/sh\nwhile true; do echo mock-agent alive; sleep 1; done\n"
	if err := os.WriteFile(mockBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	token := "test-integration-token"
	t.Setenv("AGENTSD_TOKEN", token)

	absWS, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	absJobs, err := filepath.Abs(jobs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(absJobs, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Listen:            "127.0.0.1:0",
		JobsDir:           absJobs,
		WorkspaceRoot:     absWS,
		DefaultCwd:        ".",
		MaxConcurrentJobs: 1,
		DefaultTimeout:    "5m",
		MaxTimeout:        "10m",
		DefaultTimeoutDur: 5 * time.Minute,
		MaxTimeoutDur:     10 * time.Minute,
		Token:             token,
		// Middleware reads TokenMap (populated by config load); set explicitly for hand-built cfg.
		TokenMap: map[string]string{"default": token},
		Auth: config.AuthConfig{
			BearerEnv: "AGENTSD_TOKEN",
		},
		Agents: map[string]config.AgentConfig{
			"mock": {
				Bin:  mockBin,
				Args: []string{},
			},
		},
		Allow: config.AllowConfig{
			Paths: []string{"."},
		},
		Caps: config.CapsConfig{
			Default:  []string{"fs_read"},
			Elevated: []string{},
		},
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := job.OpenStore(cfg.JobsDir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	mgr := job.NewManager(cfg, store, log)
	mgr.Start()
	t.Cleanup(mgr.Stop)

	sess, err := session.NewManager(cfg, log)
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}

	srv := api.New(cfg, mgr, sess, nil, log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	client := &http.Client{Timeout: 15 * time.Second}
	auth := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// health (no auth)
	res, err := client.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("healthz %d", res.StatusCode)
	}

	// create session
	body := `{"agent":"mock","cwd":".","mode":"tty"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("create session HTTP %d: %s", res.StatusCode, b)
	}
	var created struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Tmux  string `json:"tmux"`
	}
	if err := json.Unmarshal(b, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.State != "running" {
		t.Fatalf("unexpected session: %+v raw=%s", created, b)
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions/"+created.ID+"/kill", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		auth(req)
		if r, err := client.Do(req); err == nil {
			r.Body.Close()
		}
		if created.Tmux != "" {
			_ = exec.Command("tmux", "kill-session", "-t", created.Tmux).Run()
		}
	})

	// list
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/v1/sessions", nil)
	auth(req)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("list %d: %s", res.StatusCode, b)
	}
	if !strings.Contains(string(b), created.ID) {
		t.Fatalf("list missing session: %s", b)
	}

	// tmux session exists
	if err := exec.Command("tmux", "has-session", "-t", created.Tmux).Run(); err != nil {
		t.Fatalf("tmux has-session %s: %v", created.Tmux, err)
	}

	// kill
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions/"+created.ID+"/kill", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("kill %d: %s", res.StatusCode, b)
	}
	if err := exec.Command("tmux", "has-session", "-t", created.Tmux).Run(); err == nil {
		t.Fatalf("tmux session still alive after kill")
	}

	// resume restarts tmux under same id
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions/"+created.ID+"/resume", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("resume %d: %s", res.StatusCode, b)
	}
	var resumed struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Tmux  string `json:"tmux"`
	}
	if err := json.Unmarshal(b, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.ID != created.ID || resumed.State != "running" {
		t.Fatalf("unexpected resume: %+v", resumed)
	}
	if err := exec.Command("tmux", "has-session", "-t", resumed.Tmux).Run(); err != nil {
		t.Fatalf("tmux after resume: %v", err)
	}

	// resume while live is idempotent
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions/"+created.ID+"/resume", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("resume-live %d: %s", res.StatusCode, b)
	}
}
