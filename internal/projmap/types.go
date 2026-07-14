// Package projmap builds durable project maps for agent orientation.
package projmap

import "time"

const (
	// DirName is the workspace-relative directory for map artifacts.
	DirName = ".agents"
	// MapMarkdown is the human/agent-readable map.
	MapMarkdown = "PROJECT_MAP.md"
	// MapJSON is the structured map.
	MapJSON = "project_map.json"
	// MapMeta is generation metadata (staleness).
	MapMeta = "map_meta.json"
	// ToolVersion bumps when map format changes.
	ToolVersion = "1"
)

// Meta is written to map_meta.json for staleness checks.
type Meta struct {
	GeneratedAt time.Time `json:"generated_at"`
	GitHead     string    `json:"git_head,omitempty"`
	GitBranch   string    `json:"git_branch,omitempty"`
	ToolVersion string    `json:"tool_version"`
	Root        string    `json:"root"`
	RelCwd      string    `json:"rel_cwd"`
}

// Map is the structured project overview.
type Map struct {
	Root        string    `json:"root"`
	RelCwd      string    `json:"rel_cwd"`
	GeneratedAt time.Time `json:"generated_at"`
	GitHead     string    `json:"git_head,omitempty"`
	GitBranch   string    `json:"git_branch,omitempty"`
	GitRemote   string    `json:"git_remote,omitempty"`
	Stack       []string  `json:"stack,omitempty"`
	ReadFirst   []string  `json:"read_first,omitempty"`
	Entrypoints []string  `json:"entrypoints,omitempty"`
	// Top-level and important dirs (summarized).
	Layout []DirSummary `json:"layout,omitempty"`
	// Short notes / heuristics.
	Notes []string `json:"notes,omitempty"`
}

// DirSummary describes one directory in the layout.
type DirSummary struct {
	Path      string   `json:"path"`
	Files     int      `json:"files"`
	Dirs      int      `json:"dirs"`
	Sample    []string `json:"sample,omitempty"` // up to N file names
	Important bool     `json:"important,omitempty"`
}

// Status reports whether a map exists and if it looks stale.
type Status struct {
	Exists      bool      `json:"exists"`
	Path        string    `json:"path,omitempty"`
	GeneratedAt time.Time `json:"generated_at,omitempty"`
	GitHead     string    `json:"git_head,omitempty"`
	CurrentHead string    `json:"current_head,omitempty"`
	Stale       bool      `json:"stale"`
	Reason      string    `json:"reason,omitempty"`
	ToolVersion string    `json:"tool_version,omitempty"`
}
