package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/reloadlife/agents/internal/projmap"
	"github.com/reloadlife/agents/internal/redact"
)

// IndexOptions controls workspace indexing.
type IndexOptions struct {
	// Clear existing docs for this workspace before index.
	Clear bool
	// IncludeCode adds a shallow sample of source files (off by default).
	IncludeCode bool
}

// IndexWorkspace indexes map + docs for absRoot under workspace key wsKey.
func (s *Store) IndexWorkspace(wsKey, absRoot string, opt IndexOptions) (n int, err error) {
	if opt.Clear {
		if _, err := s.ClearWorkspace(wsKey); err != nil {
			return 0, err
		}
	}
	// project map
	if md, err := projmap.ReadMarkdown(absRoot); err == nil && md != "" {
		if _, err := s.Upsert(Doc{
			Workspace: wsKey,
			Path:      filepath.Join(projmap.DirName, projmap.MapMarkdown),
			Title:     "PROJECT_MAP",
			Source:    "map",
			Text:      redact.Line(md),
		}); err != nil {
			return n, err
		}
		n++
	}
	// standard docs
	globs := []string{
		"README.md", "README", "AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md",
		"SECURITY.md", "CHANGELOG.md",
		"docs/*.md", "docs/**/*.md",
	}
	seen := map[string]bool{}
	for _, g := range globs {
		matches, _ := filepath.Glob(filepath.Join(absRoot, g))
		// also walk docs/ one level if glob weak on **
		if strings.Contains(g, "**") {
			_ = filepath.WalkDir(filepath.Join(absRoot, "docs"), func(path string, d os.DirEntry, err error) error {
				if err != nil || d == nil || d.IsDir() {
					return nil
				}
				if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
					matches = append(matches, path)
				}
				return nil
			})
		}
		for _, abs := range matches {
			rel, err := filepath.Rel(absRoot, abs)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if seen[rel] {
				continue
			}
			seen[rel] = true
			if err := s.indexFile(wsKey, absRoot, rel, "doc"); err != nil {
				continue
			}
			n++
		}
	}
	if opt.IncludeCode {
		// shallow: only top-level and cmd/internal sample — avoid huge trees
		for _, sub := range []string{".", "cmd", "internal", "src", "app"} {
			dir := filepath.Join(absRoot, sub)
			ents, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if e.IsDir() || !isCodeFile(e.Name()) {
					continue
				}
				rel := e.Name()
				if sub != "." {
					rel = filepath.ToSlash(filepath.Join(sub, e.Name()))
				}
				if seen[rel] {
					continue
				}
				seen[rel] = true
				if err := s.indexFile(wsKey, absRoot, rel, "code"); err != nil {
					continue
				}
				n++
			}
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("nothing indexed under %s (generate a project map or add README/docs)", absRoot)
	}
	return n, nil
}

func (s *Store) indexFile(wsKey, absRoot, rel, source string) error {
	// drop prior chunks for this path
	_, _ = s.deletePathPrefix(wsKey, rel)
	abs := filepath.Join(absRoot, filepath.FromSlash(rel))
	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	// skip huge files
	if len(b) > 256*1024 {
		b = b[:256*1024]
	}
	text := redact.Line(string(b))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// chunk large docs
	chunks := chunkText(text, 4000)
	for i, ch := range chunks {
		title := filepath.Base(rel)
		path := rel
		if len(chunks) > 1 {
			path = fmt.Sprintf("%s#chunk-%d", rel, i)
			title = fmt.Sprintf("%s (%d/%d)", title, i+1, len(chunks))
		}
		if _, err := s.Upsert(Doc{
			Workspace: wsKey,
			Path:      path,
			Title:     title,
			Source:    source,
			Text:      ch,
		}); err != nil {
			return err
		}
	}
	return nil
}

func isCodeFile(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".md"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func chunkText(s string, max int) []string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return []string{s}
	}
	var out []string
	for len(s) > 0 {
		if len(s) <= max {
			out = append(out, s)
			break
		}
		// break on newline near max
		cut := max
		if i := strings.LastIndex(s[:max], "\n\n"); i > max/2 {
			cut = i
		} else if i := strings.LastIndex(s[:max], "\n"); i > max/2 {
			cut = i
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	return out
}
