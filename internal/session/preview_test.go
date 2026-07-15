package session

import (
	"strings"
	"testing"
	"time"
)

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m plain"
	got := StripANSI(in)
	if got != "red plain" {
		t.Fatalf("got %q", got)
	}
}

func TestTailLines(t *testing.T) {
	in := []byte("a\nb\nc\nd\ne\n")
	got := string(tailLines(in, 3))
	if got != "c\nd\ne\n" {
		t.Fatalf("got %q", got)
	}
	// fewer lines than n
	got = string(tailLines([]byte("only\n"), 5))
	if got != "only\n" {
		t.Fatalf("got %q", got)
	}
}

func TestShortHashStable(t *testing.T) {
	a := shortHash("hello")
	b := shortHash("hello")
	c := shortHash("world")
	if a != b || len(a) != 16 {
		t.Fatalf("hash a=%q b=%q", a, b)
	}
	if a == c {
		t.Fatal("expected different hashes")
	}
}

func TestNoteActivityBusyIdle(t *testing.T) {
	m := &Manager{activity: map[string]activitySample{}}
	// first sample → unknown
	h := m.noteActivity("s1", "aaa", true)
	if h != ActivityUnknown {
		t.Fatalf("first: %s", h)
	}
	// change → busy
	h = m.noteActivity("s1", "bbb", true)
	if h != ActivityBusy {
		t.Fatalf("change: %s", h)
	}
	// same hash, within window → busy (recent change time was just set)
	h = m.noteActivity("s1", "bbb", true)
	if h != ActivityBusy {
		t.Fatalf("stable short: %s", h)
	}
	// age the sample past idle threshold
	m.mu.Lock()
	m.activity["s1"] = activitySample{hash: "bbb", at: time.Now().Add(-20 * time.Second)}
	m.mu.Unlock()
	h = m.noteActivity("s1", "bbb", true)
	if h != ActivityIdle {
		t.Fatalf("idle: %s", h)
	}
}

func TestSnippetStripIntegration(t *testing.T) {
	// ensure strip + tail works on multi-line ANSI
	raw := "\x1b[1mline1\x1b[0m\nline2\nline3\n"
	plain := StripANSI(raw)
	if strings.Contains(plain, "\x1b") {
		t.Fatal("ansi remains")
	}
	tail := string(tailLines([]byte(plain), 2))
	if !strings.Contains(tail, "line2") || !strings.Contains(tail, "line3") {
		t.Fatalf("tail %q", tail)
	}
}
