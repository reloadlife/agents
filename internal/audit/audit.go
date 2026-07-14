// Package audit appends an append-only JSONL audit log of control-plane actions.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one audit line.
type Entry struct {
	At     time.Time      `json:"at"`
	Action string         `json:"action"`
	Actor  string         `json:"actor,omitempty"` // token label if multi-token
	Target string         `json:"target,omitempty"`
	Detail map[string]any `json:"detail,omitempty"`
	IP     string         `json:"ip,omitempty"`
}

// Log writes JSONL under jobs_dir/audit/audit.jsonl
type Log struct {
	path string
	mu   sync.Mutex
}

func New(jobsDir string) (*Log, error) {
	dir := filepath.Join(jobsDir, "audit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Log{path: filepath.Join(dir, "audit.jsonl")}, nil
}

func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Log) Record(action, actor, target, ip string, detail map[string]any) {
	if l == nil || action == "" {
		return
	}
	e := Entry{
		At:     time.Now().UTC(),
		Action: action,
		Actor:  actor,
		Target: target,
		Detail: detail,
		IP:     ip,
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(b)
	_ = f.Close()
}

// Tail returns the last n entries (newest last).
func (l *Log) Tail(n int) ([]Entry, error) {
	if l == nil {
		return nil, nil
	}
	if n <= 0 {
		n = 100
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := splitNonEmpty(string(b))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var out []Entry
	for _, line := range lines {
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func splitNonEmpty(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
