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
	"github.com/reloadlife/agents/internal/redact"
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

// SearchHit is a recording match with a text snippet around the query.
type SearchHit struct {
	Meta
	Snippet string `json:"snippet,omitempty"`
}

// Retention limits how many / how old recordings are kept per session.
type Retention struct {
	// MaxPerSession keeps the newest N recordings per session (0 = default 20, <0 = unlimited).
	MaxPerSession int
	// MaxAgeDays drops recordings older than N days (0 = disabled).
	MaxAgeDays int
}

// DefaultMaxPerSession is used when Retention.MaxPerSession is 0.
const DefaultMaxPerSession = 20

// Store keeps recordings under dir.
type Store struct {
	dir       string
	mu        sync.Mutex
	retention Retention
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, retention: Retention{MaxPerSession: DefaultMaxPerSession}}, nil
}

// SetRetention configures prune-on-archive limits.
func (s *Store) SetRetention(r Retention) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retention = r
}

func (s *Store) Dir() string { return s.dir }

// Archive writes a pane snapshot + meta. Best-effort; empty data is skipped.
// Pane text is redacted line-by-line and ANSI-stripped for safer storage.
// After write, retention pruning runs for that session.
func (s *Store) Archive(sessionID, agent, cwd, name, reason string, data []byte) (*Meta, error) {
	if s == nil || len(data) == 0 || sessionID == "" {
		return nil, nil
	}
	// Redact secrets then strip ANSI before persisting.
	data = redactPane(data)

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
	// prune under same lock
	s.pruneSessionLocked(sessDir)
	return &meta, nil
}

// redactPane applies secret redaction per line, then strips ANSI.
func redactPane(data []byte) []byte {
	text := string(data)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = redact.Line(line)
	}
	return []byte(StripANSI(strings.Join(lines, "\n")))
}

// pruneSessionLocked enforces retention for one session directory. Caller holds mu.
func (s *Store) pruneSessionLocked(sessDir string) {
	ret := s.retention
	maxN := ret.MaxPerSession
	if maxN == 0 {
		maxN = DefaultMaxPerSession
	}
	metas := s.readSessDir(sessDir)
	if len(metas) == 0 {
		return
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].CreatedAt.After(metas[j].CreatedAt)
	})

	var cutoff time.Time
	if ret.MaxAgeDays > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -ret.MaxAgeDays)
	}

	keep := 0
	for i, m := range metas {
		drop := false
		if maxN > 0 && i >= maxN {
			drop = true
		}
		if !cutoff.IsZero() && m.CreatedAt.Before(cutoff) {
			drop = true
		}
		if drop {
			_ = os.Remove(filepath.Join(sessDir, m.ID+".pane"))
			_ = os.Remove(filepath.Join(sessDir, m.ID+".json"))
			continue
		}
		keep++
	}
	_ = keep
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

// Delete removes a recording by id.
func (s *Store) Delete(id string) error {
	if s == nil || id == "" {
		return fmt.Errorf("not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(s.dir, e.Name(), id+".json")
		if _, err := os.Stat(metaPath); err != nil {
			continue
		}
		_ = os.Remove(filepath.Join(s.dir, e.Name(), id+".pane"))
		if err := os.Remove(metaPath); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("recording not found")
}

// Search scans pane text (case-insensitive substring). Limit results.
func (s *Store) Search(query string, limit int) ([]Meta, error) {
	hits, err := s.SearchWithSnippets(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Meta, len(hits))
	for i, h := range hits {
		out[i] = h.Meta
	}
	return out, nil
}

// SearchWithSnippets is like Search but includes a text snippet around each match.
func (s *Store) SearchWithSnippets(query string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		list, err := s.List("", limit)
		if err != nil {
			return nil, err
		}
		out := make([]SearchHit, len(list))
		for i, m := range list {
			out[i] = SearchHit{Meta: m}
		}
		return out, nil
	}
	if limit <= 0 {
		limit = 50
	}
	all, err := s.List("", 500)
	if err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, m := range all {
		_, pane, err := s.Get(m.ID)
		if err != nil {
			continue
		}
		plain := StripANSI(string(pane))
		lower := strings.ToLower(plain)
		metaMatch := strings.Contains(strings.ToLower(m.Name+" "+m.Agent+" "+m.Cwd), query)
		idx := strings.Index(lower, query)
		if idx < 0 && !metaMatch {
			continue
		}
		snippet := ""
		if idx >= 0 {
			snippet = snippetAround(plain, idx, len(query))
		} else if plain != "" {
			// meta-only match: short head of pane
			snippet = plain
			if len(snippet) > 120 {
				snippet = snippet[:120] + "…"
			}
		}
		hits = append(hits, SearchHit{Meta: m, Snippet: snippet})
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}

// snippetAround extracts context around a match index in plain text.
func snippetAround(plain string, idx, qlen int) string {
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + qlen + 60
	if end > len(plain) {
		end = len(plain)
	}
	// snap to rune-safe by working on bytes carefully — plain is UTF-8; simple slice is fine for ASCII-heavy terminals
	sn := plain[start:end]
	sn = strings.ReplaceAll(sn, "\n", " ")
	sn = strings.TrimSpace(sn)
	if start > 0 {
		sn = "…" + sn
	}
	if end < len(plain) {
		sn = sn + "…"
	}
	return sn
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

// StripANSI removes common CSI ANSI escape sequences from s.
func StripANSI(s string) string {
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
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']' {
			i += 2
			for i < len(s) {
				if s[i] == 0x07 {
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
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
