// Package recording archives session terminal snapshots for later browse/replay.
// Opt-in via sessions.recording (default false for privacy).
package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Meta describes one archived capture.
type Meta struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	Cwd       string    `json:"cwd,omitempty"`
	Name      string    `json:"name,omitempty"`
	Bytes     int       `json:"bytes"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason,omitempty"` // kill|detach|periodic|manual
}

// Store keeps recordings under dir.
type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

// Archive writes a pane snapshot + meta. Best-effort; empty data is skipped.
func (s *Store) Archive(sessionID, agent, cwd, name, reason string, data []byte) (*Meta, error) {
	if s == nil || len(data) == 0 || sessionID == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := "r_" + ulid.Make().String()
	sessDir := filepath.Join(s.dir, sanitize(sessionID))
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		return nil, err
	}
	meta := Meta{
		ID:        id,
		SessionID: sessionID,
		Agent:     agent,
		Cwd:       cwd,
		Name:      name,
		Bytes:     len(data),
		CreatedAt: time.Now().UTC(),
		Reason:    reason,
	}
	if err := os.WriteFile(filepath.Join(sessDir, id+".pane"), data, 0o600); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(sessDir, id+".json"), b, 0o600); err != nil {
		return nil, err
	}
	return &meta, nil
}

// List returns metas newest-first. Optional sessionID filter.
func (s *Store) List(sessionID string, limit int) ([]Meta, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Meta
	if sessionID != "" {
		out = append(out, s.readSessDir(filepath.Join(s.dir, sanitize(sessionID)))...)
	} else {
		ents, err := os.ReadDir(s.dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			out = append(out, s.readSessDir(filepath.Join(s.dir, e.Name()))...)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) readSessDir(dir string) []Meta {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Meta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(b, &m) == nil && m.ID != "" {
			out = append(out, m)
		}
	}
	return out
}

// Get returns meta + pane bytes.
func (s *Store) Get(id string) (*Meta, []byte, error) {
	if s == nil || id == "" {
		return nil, nil, fmt.Errorf("not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// search
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(s.dir, e.Name(), id+".json")
		b, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		pane, err := os.ReadFile(filepath.Join(s.dir, e.Name(), id+".pane"))
		if err != nil {
			return &m, nil, err
		}
		return &m, pane, nil
	}
	return nil, nil, fmt.Errorf("recording not found")
}

// Search scans pane text (case-insensitive substring). Limit results.
func (s *Store) Search(query string, limit int) ([]Meta, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return s.List("", limit)
	}
	if limit <= 0 {
		limit = 50
	}
	all, err := s.List("", 500)
	if err != nil {
		return nil, err
	}
	var hits []Meta
	for _, m := range all {
		_, pane, err := s.Get(m.ID)
		if err != nil {
			continue
		}
		// strip some ANSI for search
		plain := stripANSI(string(pane))
		if strings.Contains(strings.ToLower(plain), query) ||
			strings.Contains(strings.ToLower(m.Name+" "+m.Agent+" "+m.Cwd), query) {
			hits = append(hits, m)
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits, nil
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, s)
	if s == "" {
		return "unknown"
	}
	return s
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) {
				c := s[i]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
