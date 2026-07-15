package tui

import (
	"fmt"
	"path"
	"strings"
)

// sessionItem is a list row for GET /v1/sessions entries.
type sessionItem struct {
	id, name, agent, cwd, state, tmux, branch string
	worktree                                  bool
}

func (s sessionItem) Title() string {
	label := s.id
	if s.name != "" {
		label = s.name + " · " + shortID(s.id)
	} else {
		label = shortID(s.id)
		if label == "" {
			label = s.id
		}
	}
	st := s.state
	if st == "" {
		st = "?"
	}
	return fmt.Sprintf("%s  [%s]", label, st)
}

func (s sessionItem) Description() string {
	base := path.Base(strings.TrimRight(s.cwd, "/"))
	if base == "" || base == "." {
		base = s.cwd
	}
	parts := []string{s.agent, base}
	if s.branch != "" {
		parts = append(parts, s.branch)
	}
	if s.worktree {
		parts = append(parts, "wt")
	}
	if s.tmux != "" {
		parts = append(parts, s.tmux)
	}
	return strings.Join(parts, " · ")
}

func (s sessionItem) FilterValue() string {
	base := path.Base(s.cwd)
	return strings.Join([]string{
		s.id, s.name, s.agent, s.cwd, base, s.branch, s.state, s.tmux,
		boolTag(s.worktree, "worktree wt"),
	}, " ")
}

func boolTag(on bool, words string) string {
	if on {
		return words
	}
	return ""
}

func shortID(id string) string {
	if len(id) <= 16 {
		return id
	}
	// s_01ABCDEF… → keep prefix + tail
	if strings.HasPrefix(id, "s_") && len(id) > 14 {
		return id[:10] + "…" + id[len(id)-4:]
	}
	return id[:8] + "…"
}

func shortPath(p string) string {
	if len(p) <= 24 {
		return p
	}
	return "…" + p[len(p)-22:]
}

func cwdBase(p string) string {
	b := path.Base(strings.TrimRight(p, "/"))
	if b == "" || b == "." {
		return p
	}
	return b
}
