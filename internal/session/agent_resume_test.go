package session

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reloadlife/agents/internal/config"
)

func TestTTYLaunchArgs_Grok(t *testing.T) {
	acfg := config.AgentConfig{Bin: "grok", Args: nil}

	got := ttyLaunchArgs("grok", acfg, "abc-uuid", false)
	if len(got) != 2 || got[0] != "--session-id" || got[1] != "abc-uuid" {
		t.Fatalf("create pin: %v", got)
	}

	got = ttyLaunchArgs("grok", acfg, "abc-uuid", true)
	if len(got) != 2 || got[0] != "--resume" || got[1] != "abc-uuid" {
		t.Fatalf("resume id: %v", got)
	}

	got = ttyLaunchArgs("grok", acfg, "", true)
	if len(got) != 1 || got[0] != "--continue" {
		t.Fatalf("resume fallback: %v", got)
	}
}

func TestTTYLaunchArgs_ClaudeCodex(t *testing.T) {
	claude := ttyLaunchArgs("claude", config.AgentConfig{}, "sess-1", true)
	if len(claude) != 2 || claude[0] != "--resume" || claude[1] != "sess-1" {
		t.Fatalf("claude resume: %v", claude)
	}
	codex := ttyLaunchArgs("codex", config.AgentConfig{}, "sess-2", true)
	if len(codex) != 2 || codex[0] != "resume" || codex[1] != "sess-2" {
		t.Fatalf("codex resume: %v", codex)
	}
	codexLast := ttyLaunchArgs("codex", config.AgentConfig{}, "", true)
	if len(codexLast) != 2 || codexLast[0] != "resume" || codexLast[1] != "--last" {
		t.Fatalf("codex last: %v", codexLast)
	}
}

func TestTTYLaunchArgs_ConfigOverride(t *testing.T) {
	acfg := config.AgentConfig{
		ResumeArgs:    []string{"--resume", "{id}", "--fullscreen"},
		SessionIDArgs: []string{"--session-id", "{id}"},
	}
	got := ttyLaunchArgs("custom", acfg, "x1", true)
	if len(got) != 3 || got[1] != "x1" || got[2] != "--fullscreen" {
		t.Fatalf("override resume: %v", got)
	}
	got = ttyLaunchArgs("custom", acfg, "x1", false)
	if len(got) != 2 || got[0] != "--session-id" || got[1] != "x1" {
		t.Fatalf("override create: %v", got)
	}
}

func TestDiscoverGrokSessionID(t *testing.T) {
	home := t.TempDir()
	cwd := "/root/workspace/demo"
	base := filepath.Join(home, ".grok", "sessions", url.QueryEscape(cwd))
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	old := "11111111-1111-1111-1111-111111111111"
	newer := "22222222-2222-2222-2222-222222222222"
	if err := os.Mkdir(filepath.Join(base, old), 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.Mkdir(filepath.Join(base, newer), 0o755); err != nil {
		t.Fatal(err)
	}
	// touch newer more recently
	now := time.Now()
	_ = os.Chtimes(filepath.Join(base, newer), now, now)

	got := discoverGrokSessionID(home, cwd, time.Time{})
	if got != newer {
		t.Fatalf("want %s got %s", newer, got)
	}
}

func TestCanPinSessionID(t *testing.T) {
	if !canPinSessionID("grok") {
		t.Fatal("grok should pin")
	}
	if canPinSessionID("claude") {
		t.Fatal("claude pin not supported yet")
	}
}
