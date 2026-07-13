package workspaces

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// Entry is a cwd a client may use for sessions.
type Entry struct {
	Path    string `json:"path"` // relative to workspace_root
	Abs     string `json:"abs,omitempty"`
	Exists  bool   `json:"exists"`
	IsDir   bool   `json:"is_dir"`
	Default bool   `json:"default,omitempty"`
}

// List returns concrete directories under workspace that match the allowlist.
// Patterns like "foo/*" expand to immediate children of foo that exist.
func List(cfg *config.Config) []Entry {
	root := cfg.WorkspaceRoot
	patterns := cfg.Allow.Paths
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	seen := map[string]bool{}
	var out []Entry

	add := func(rel string) {
		rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
		if rel == "" {
			rel = "."
		}
		if seen[rel] {
			return
		}
		// must pass allowlist
		abs, err := pathallow.Resolve(root, rel, patterns)
		if err != nil {
			return
		}
		st, err := os.Stat(abs)
		e := Entry{Path: rel, Abs: abs, Exists: err == nil}
		if err == nil {
			e.IsDir = st.IsDir()
		}
		if !e.Exists || !e.IsDir {
			return
		}
		if rel == cfg.DefaultCwd || (cfg.DefaultCwd == "" && rel == ".") {
			e.Default = true
		}
		seen[rel] = true
		out = append(out, e)
	}

	for _, p := range patterns {
		p = filepath.ToSlash(strings.TrimSpace(p))
		switch {
		case p == "." || p == "*" || p == "**":
			add(".")
			// also list top-level dirs
			ents, _ := os.ReadDir(root)
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					add(e.Name())
				}
			}
		case strings.HasSuffix(p, "/**"):
			base := strings.TrimSuffix(p, "/**")
			add(base)
		case strings.HasSuffix(p, "/*"):
			base := strings.TrimSuffix(p, "/*")
			add(base)
			dir := filepath.Join(root, filepath.FromSlash(base))
			ents, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					add(base + "/" + e.Name())
				}
			}
		default:
			add(p)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return out[i].Path < out[j].Path
	})
	return out
}
