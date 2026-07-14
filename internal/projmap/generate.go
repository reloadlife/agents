package projmap

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDepth        = 4
	maxSampleFiles  = 12
	maxLayoutDirs   = 80
	maxReadFirst    = 16
	maxEntrypoints  = 24
)

// Generate walks absRoot and returns a Map (does not write files).
func Generate(absRoot, relCwd string) (*Map, error) {
	absRoot = filepath.Clean(absRoot)
	st, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", absRoot)
	}

	now := time.Now().UTC()
	m := &Map{
		Root:        absRoot,
		RelCwd:      relCwd,
		GeneratedAt: now,
	}
	fillGit(m, absRoot)
	m.Stack = detectStack(absRoot)
	m.ReadFirst = detectReadFirst(absRoot)
	m.Entrypoints = detectEntrypoints(absRoot)
	layout, err := walkLayout(absRoot)
	if err != nil {
		return nil, err
	}
	m.Layout = layout
	m.Notes = buildNotes(m)
	return m, nil
}

// WriteArtifacts writes PROJECT_MAP.md, project_map.json, map_meta.json under absRoot/.agents/.
func WriteArtifacts(absRoot, relCwd string, m *Map) (metaPath string, err error) {
	dir := filepath.Join(absRoot, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	jb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, MapJSON), jb, 0o644); err != nil {
		return "", err
	}
	md := RenderMarkdown(m)
	if err := os.WriteFile(filepath.Join(dir, MapMarkdown), []byte(md), 0o644); err != nil {
		return "", err
	}
	meta := Meta{
		GeneratedAt: m.GeneratedAt,
		GitHead:     m.GitHead,
		GitBranch:   m.GitBranch,
		ToolVersion: ToolVersion,
		Root:        absRoot,
		RelCwd:      relCwd,
	}
	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	metaPath = filepath.Join(dir, MapMeta)
	if err := os.WriteFile(metaPath, mb, 0o644); err != nil {
		return "", err
	}
	return metaPath, nil
}

// GenerateAndWrite builds and persists map artifacts.
func GenerateAndWrite(absRoot, relCwd string) (*Map, string, error) {
	m, err := Generate(absRoot, relCwd)
	if err != nil {
		return nil, "", err
	}
	metaPath, err := WriteArtifacts(absRoot, relCwd, m)
	if err != nil {
		return nil, "", err
	}
	return m, metaPath, nil
}

// ReadStatus loads map_meta and compares to current git HEAD.
func ReadStatus(absRoot string) Status {
	metaPath := filepath.Join(absRoot, DirName, MapMeta)
	mdPath := filepath.Join(absRoot, DirName, MapMarkdown)
	st := Status{Path: mdPath}
	b, err := os.ReadFile(metaPath)
	if err != nil {
		if _, e2 := os.Stat(mdPath); e2 == nil {
			st.Exists = true
			st.Stale = true
			st.Reason = "map exists but map_meta.json missing"
			return st
		}
		st.Reason = "no project map"
		return st
	}
	var meta Meta
	if err := json.Unmarshal(b, &meta); err != nil {
		st.Exists = true
		st.Stale = true
		st.Reason = "invalid map_meta.json"
		return st
	}
	st.Exists = true
	st.GeneratedAt = meta.GeneratedAt
	st.GitHead = meta.GitHead
	st.ToolVersion = meta.ToolVersion
	st.CurrentHead = gitRevParse(absRoot)
	if meta.ToolVersion != "" && meta.ToolVersion != ToolVersion {
		st.Stale = true
		st.Reason = "map tool version mismatch"
		return st
	}
	if meta.GitHead != "" && st.CurrentHead != "" && meta.GitHead != st.CurrentHead {
		st.Stale = true
		st.Reason = "git HEAD changed since map generation"
		return st
	}
	// age heuristic: 14 days
	if !meta.GeneratedAt.IsZero() && time.Since(meta.GeneratedAt) > 14*24*time.Hour {
		st.Stale = true
		st.Reason = "map older than 14 days"
		return st
	}
	return st
}

// ReadMarkdown returns PROJECT_MAP.md contents if present.
func ReadMarkdown(absRoot string) (string, error) {
	b, err := os.ReadFile(filepath.Join(absRoot, DirName, MapMarkdown))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fillGit(m *Map, root string) {
	m.GitHead = gitRevParse(root)
	m.GitBranch = strings.TrimSpace(runGit(root, "rev-parse", "--abbrev-ref", "HEAD"))
	m.GitRemote = strings.TrimSpace(runGit(root, "config", "--get", "remote.origin.url"))
}

func gitRevParse(root string) string {
	return strings.TrimSpace(runGit(root, "rev-parse", "HEAD"))
}

func runGit(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func detectStack(root string) []string {
	type signal struct {
		file string
		name string
	}
	signals := []signal{
		{"go.mod", "Go"},
		{"package.json", "Node/JS"},
		{"pnpm-workspace.yaml", "pnpm workspace"},
		{"yarn.lock", "Yarn"},
		{"bun.lock", "Bun"},
		{"bun.lockb", "Bun"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Cargo.toml", "Rust"},
		{"composer.json", "PHP"},
		{"Gemfile", "Ruby"},
		{"pom.xml", "Java/Maven"},
		{"build.gradle", "Java/Gradle"},
		{"build.gradle.kts", "Java/Gradle"},
		{"Makefile", "Make"},
		{"Dockerfile", "Docker"},
		{"docker-compose.yml", "Docker Compose"},
		{"docker-compose.yaml", "Docker Compose"},
		{"wrangler.toml", "Cloudflare Workers"},
		{"wrangler.jsonc", "Cloudflare Workers"},
		{"Cargo.lock", "Rust"},
		{"mix.exs", "Elixir"},
	}
	var out []string
	seen := map[string]bool{}
	for _, s := range signals {
		if _, err := os.Stat(filepath.Join(root, s.file)); err == nil {
			if !seen[s.name] {
				seen[s.name] = true
				out = append(out, s.name)
			}
		}
	}
	return out
}

func detectReadFirst(root string) []string {
	candidates := []string{
		"README.md", "README", "README.txt",
		"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CODEX.md",
		filepath.Join(DirName, MapMarkdown),
		"CONTRIBUTING.md", "docs/ARCHITECTURE.md", "docs/README.md",
		"go.mod", "package.json", "pyproject.toml", "Cargo.toml",
	}
	var out []string
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(root, c)); err == nil {
			out = append(out, filepath.ToSlash(c))
			if len(out) >= maxReadFirst {
				break
			}
		}
	}
	return out
}

func detectEntrypoints(root string) []string {
	var out []string
	// Go main packages under cmd/
	cmdDir := filepath.Join(root, "cmd")
	if ents, err := os.ReadDir(cmdDir); err == nil {
		for _, e := range ents {
			if e.IsDir() && !shouldSkipDir(e.Name()) {
				mainGo := filepath.Join(cmdDir, e.Name(), "main.go")
				if _, err := os.Stat(mainGo); err == nil {
					out = append(out, filepath.ToSlash(filepath.Join("cmd", e.Name(), "main.go")))
				}
			}
		}
	}
	// common single-file mains
	for _, p := range []string{"main.go", "main.py", "src/main.rs", "src/index.ts", "src/index.js", "src/main.ts", "app/main.py"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			out = append(out, filepath.ToSlash(p))
		}
	}
	// package.json scripts hint
	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Main    string            `json:"main"`
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			if pkg.Main != "" {
				out = append(out, "package.json#main:"+pkg.Main)
			}
			for _, name := range []string{"dev", "start", "build", "test"} {
				if _, ok := pkg.Scripts[name]; ok {
					out = append(out, "package.json#scripts."+name)
				}
			}
		}
	}
	if len(out) > maxEntrypoints {
		out = out[:maxEntrypoints]
	}
	return out
}

func walkLayout(root string) ([]DirSummary, error) {
	var out []DirSummary
	// always include root summary
	rootSum, err := summarizeDir(root, ".", 0)
	if err != nil {
		return nil, err
	}
	out = append(out, rootSum)

	var walk func(abs, rel string, depth int) error
	walk = func(abs, rel string, depth int) error {
		if depth >= maxDepth || len(out) >= maxLayoutDirs {
			return nil
		}
		ents, err := os.ReadDir(abs)
		if err != nil {
			return nil // skip unreadable
		}
		// sort for stability
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if shouldSkipDir(name) {
				continue
			}
			childRel := name
			if rel != "." {
				childRel = filepath.Join(rel, name)
			}
			childAbs := filepath.Join(abs, name)
			sum, err := summarizeDir(childAbs, childRel, depth+1)
			if err != nil {
				continue
			}
			sum.Important = isImportantDir(childRel)
			out = append(out, sum)
			if sum.Important || depth < 2 {
				if err := walk(childAbs, childRel, depth+1); err != nil {
					return err
				}
			}
			if len(out) >= maxLayoutDirs {
				return nil
			}
		}
		return nil
	}
	_ = walk(root, ".", 0)
	return out, nil
}

func summarizeDir(abs, rel string, _ int) (DirSummary, error) {
	ents, err := os.ReadDir(abs)
	if err != nil {
		return DirSummary{}, err
	}
	sum := DirSummary{Path: filepath.ToSlash(rel)}
	var samples []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			if shouldSkipDir(name) {
				continue
			}
			sum.Dirs++
			continue
		}
		if shouldSkipFile(name) {
			continue
		}
		sum.Files++
		if len(samples) < maxSampleFiles {
			samples = append(samples, name)
		}
	}
	sum.Sample = samples
	return sum, nil
}

func buildNotes(m *Map) []string {
	var n []string
	n = append(n, "Prefer this map + targeted reads over recursive full-tree exploration.")
	n = append(n, "Regenerate after major structural changes: agentsctl map generate -r <cwd>")
	if m.GitHead != "" {
		n = append(n, "Stale if git HEAD differs from map_meta.json git_head.")
	}
	if len(m.Stack) == 0 {
		n = append(n, "No common stack manifests detected at repo root.")
	}
	return n
}

// RenderMarkdown turns Map into PROJECT_MAP.md.
func RenderMarkdown(m *Map) string {
	var b strings.Builder
	b.WriteString("# Project map\n\n")
	b.WriteString(fmt.Sprintf("_Generated %s · tool v%s_\n\n", m.GeneratedAt.Format(time.RFC3339), ToolVersion))
	b.WriteString("## Overview\n\n")
	b.WriteString(fmt.Sprintf("- **Root:** `%s`\n", m.Root))
	if m.RelCwd != "" {
		b.WriteString(fmt.Sprintf("- **Workspace cwd:** `%s`\n", m.RelCwd))
	}
	if m.GitBranch != "" || m.GitHead != "" {
		head := m.GitHead
		if len(head) > 12 {
			head = head[:12]
		}
		b.WriteString(fmt.Sprintf("- **Git:** `%s` @ `%s`\n", m.GitBranch, head))
	}
	if m.GitRemote != "" {
		b.WriteString(fmt.Sprintf("- **Remote:** `%s`\n", m.GitRemote))
	}
	if len(m.Stack) > 0 {
		b.WriteString(fmt.Sprintf("- **Stack:** %s\n", strings.Join(m.Stack, ", ")))
	}
	b.WriteString("\n## Read these first\n\n")
	if len(m.ReadFirst) == 0 {
		b.WriteString("_No standard docs found at root._\n")
	} else {
		for _, p := range m.ReadFirst {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
	}
	if len(m.Entrypoints) > 0 {
		b.WriteString("\n## Entrypoints / signals\n\n")
		for _, p := range m.Entrypoints {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
	}
	b.WriteString("\n## Layout\n\n")
	b.WriteString("| Path | Files | Dirs | Sample |\n")
	b.WriteString("|------|------:|-----:|--------|\n")
	for _, d := range m.Layout {
		sample := strings.Join(d.Sample, ", ")
		if len(sample) > 60 {
			sample = sample[:57] + "…"
		}
		pathCell := "`" + d.Path + "`"
		if d.Important {
			pathCell = "**" + pathCell + "**"
		}
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %s |\n", pathCell, d.Files, d.Dirs, sample))
	}
	if len(m.Notes) > 0 {
		b.WriteString("\n## Notes for agents\n\n")
		for _, n := range m.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", n))
		}
	}
	b.WriteString("\n---\n\n")
	b.WriteString("Regenerate: `agentsctl map generate -r <cwd>` or `POST /v1/maps`.\n")
	return b.String()
}
