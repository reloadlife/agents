package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ActivityHeuristic classifies recent pane content change.
type ActivityHeuristic string

const (
	ActivityBusy    ActivityHeuristic = "busy"
	ActivityIdle    ActivityHeuristic = "idle"
	ActivityUnknown ActivityHeuristic = "unknown"
)

// activitySample tracks last content hash for busy/idle detection.
type activitySample struct {
	hash string
	at   time.Time
}

// idleStableAfter is how long content must be unchanged to report idle.
const idleStableAfter = 15 * time.Second

// PreviewResult is the JSON shape for GET /v1/sessions/{id}/preview.
type PreviewResult struct {
	ID         string         `json:"id"`
	State      State          `json:"state"`
	Source     string         `json:"source"` // live|snapshot
	CapturedAt time.Time      `json:"captured_at"`
	Bytes      int            `json:"bytes"`
	Lines      int            `json:"lines"`
	Text       string         `json:"text"`
	Activity   PreviewActivity `json:"activity"`
}

// PreviewActivity is the busy/idle heuristic for a session pane.
type PreviewActivity struct {
	Heuristic   ActivityHeuristic `json:"heuristic"`
	TmuxAlive   bool              `json:"tmux_alive"`
	ContentHash string            `json:"content_hash"`
}

// ActivitySummary is a lightweight activity row for list endpoints.
type ActivitySummary struct {
	ID          string            `json:"id"`
	State       State             `json:"state"`
	Name        string            `json:"name,omitempty"`
	Agent       string            `json:"agent,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Heuristic   ActivityHeuristic `json:"heuristic"`
	TmuxAlive   bool              `json:"tmux_alive"`
	ContentHash string            `json:"content_hash,omitempty"`
	Source      string            `json:"source,omitempty"`
	Preview     string            `json:"preview,omitempty"` // last few lines, stripped
}

// Preview returns the last N lines of session output with optional ANSI strip
// and a busy/idle activity heuristic based on content-hash stability.
func (m *Manager) Preview(id string, lines int, stripANSI bool) (*PreviewResult, error) {
	if lines <= 0 {
		lines = 40
	}
	if lines > 500 {
		lines = 500
	}

	m.refreshStates()
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session not found")
	}
	cp := *s
	tmux := s.Tmux
	m.mu.Unlock()

	alive := tmux != "" && tmuxAlive(tmux)
	var data []byte
	var source string
	var err error

	if alive {
		data, err = capturePaneLast(tmux, lines)
		if err == nil && len(data) > 0 {
			source = "live"
		} else {
			// fall back to full capture + tail
			data, err = capturePane(tmux)
			if err == nil && len(data) > 0 {
				data = tailLines(data, lines)
				source = "live"
			}
		}
	}
	if source == "" {
		data, err = os.ReadFile(m.historyPath(id))
		if err != nil {
			if os.IsNotExist(err) {
				// empty preview still useful for state/activity
				data = nil
				source = "snapshot"
			} else {
				return nil, err
			}
		} else {
			data = tailLines(data, lines)
			source = "snapshot"
		}
	}

	text := string(data)
	if stripANSI {
		text = StripANSI(text)
	}
	// Normalize to exact last N lines after strip (strip can change line structure slightly).
	text = string(tailLines([]byte(text), lines))

	hash := shortHash(text)
	heuristic := m.noteActivity(id, hash, alive)

	return &PreviewResult{
		ID:         cp.ID,
		State:      cp.State,
		Source:     source,
		CapturedAt: time.Now().UTC(),
		Bytes:      len(text),
		Lines:      lines,
		Text:       text,
		Activity: PreviewActivity{
			Heuristic:   heuristic,
			TmuxAlive:   alive,
			ContentHash: hash,
		},
	}, nil
}

// ActivityList returns activity summaries for all sessions (best-effort previews).
func (m *Manager) ActivityList(previewLines int) ([]ActivitySummary, error) {
	if previewLines <= 0 {
		previewLines = 5
	}
	list, err := m.List()
	if err != nil {
		return nil, err
	}
	out := make([]ActivitySummary, 0, len(list))
	for _, s := range list {
		sum := ActivitySummary{
			ID:    s.ID,
			State: s.State,
			Name:  s.Name,
			Agent: s.Agent,
			Cwd:   s.Cwd,
		}
		pv, err := m.Preview(s.ID, previewLines, true)
		if err != nil {
			sum.Heuristic = ActivityUnknown
			out = append(out, sum)
			continue
		}
		sum.Heuristic = pv.Activity.Heuristic
		sum.TmuxAlive = pv.Activity.TmuxAlive
		sum.ContentHash = pv.Activity.ContentHash
		sum.Source = pv.Source
		sum.Preview = pv.Text
		out = append(out, sum)
	}
	return out, nil
}

// noteActivity updates the in-memory hash timeline and returns busy|idle|unknown.
func (m *Manager) noteActivity(id, hash string, tmuxAlive bool) ActivityHeuristic {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activity == nil {
		m.activity = map[string]activitySample{}
	}
	if !tmuxAlive && hash == "" {
		return ActivityUnknown
	}
	prev, ok := m.activity[id]
	if !ok {
		m.activity[id] = activitySample{hash: hash, at: now}
		if !tmuxAlive {
			return ActivityUnknown
		}
		// first sample: unknown until we see stability or change
		return ActivityUnknown
	}
	if hash != prev.hash {
		m.activity[id] = activitySample{hash: hash, at: now}
		if tmuxAlive {
			return ActivityBusy
		}
		return ActivityUnknown
	}
	// stable hash
	if now.Sub(prev.at) >= idleStableAfter {
		return ActivityIdle
	}
	// still within stability window after a change — treat as busy if live
	if tmuxAlive {
		return ActivityBusy
	}
	return ActivityUnknown
}

// capturePaneLast captures approximately the last n history lines via tmux -S -n.
func capturePaneLast(tmuxName string, n int) ([]byte, error) {
	if n < 1 {
		n = 1
	}
	out, err := exec.Command(
		"tmux", "capture-pane",
		"-t", tmuxName,
		"-e",
		"-p",
		"-S", "-"+strconv.Itoa(n),
	).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// tailLines returns the last n lines of b (including trailing newline behavior).
func tailLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return b
	}
	s := string(b)
	// Preserve whether original ended with newline by working on split lines.
	// strings.Split keeps empty trailing element if s ends with \n.
	parts := strings.Split(s, "\n")
	// If ends with newline, last element is empty — drop for counting content lines,
	// then re-join with trailing newline.
	trailingNL := strings.HasSuffix(s, "\n")
	if trailingNL && len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	out := strings.Join(parts, "\n")
	if trailingNL || len(parts) > 0 {
		// keep readable multi-line text; add trailing newline only if original had one
		if trailingNL {
			out += "\n"
		}
	}
	return []byte(out)
}

// StripANSI removes common CSI ANSI escape sequences from s.
// Duplicated here (also recording.StripANSI) to avoid import cycles.
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
		// also drop OSC sequences ESC ]
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']' {
			i += 2
			for i < len(s) {
				if s[i] == 0x07 { // BEL
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++ // skip ST
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

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
