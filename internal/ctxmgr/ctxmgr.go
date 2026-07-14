// Package ctxmgr prepares durable agent orientation for a workspace.
//
// TTY agents own their model loop — agentsd cannot inject tokens mid-turn.
// Instead we ensure files on disk (.agents/PROJECT_MAP.md, CONTEXT.md,
// INSTRUCTIONS.md) and optionally seed a short protocol into the first
// tmux keystrokes so agents start with better context, faster.
package ctxmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/memory"
	"github.com/reloadlife/agents/internal/projmap"
	"github.com/reloadlife/agents/internal/redact"
)

const (
	// ContextFile is the packed orientation blob agents should read second.
	ContextFile = "CONTEXT.md"
	// InstructionsFile is a short standing protocol under .agents/.
	InstructionsFile = "INSTRUCTIONS.md"
	// DefaultPackBudget is max characters for CONTEXT.md / pack responses.
	DefaultPackBudget = 12000
	// DefaultSeedBudget is max chars prepended into the TTY seed prompt.
	DefaultSeedBudget = 1800
)

// Options control ensure/pack behaviour.
type Options struct {
	// ForceMap regenerates the project map even when fresh.
	ForceMap bool
	// ForceIndex reindexes memory (clear+index).
	ForceIndex bool
	// IncludeCode indexes shallow code samples into memory.
	IncludeCode bool
	// AutoIndex indexes when memory is empty (default true when nil).
	AutoIndex *bool
	// WriteFiles writes CONTEXT.md + INSTRUCTIONS.md (default true when nil).
	WriteFiles *bool
	// PackBudget caps packed markdown size (default DefaultPackBudget).
	PackBudget int
	// Query optional topic for memory hits inside the pack.
	Query string
	// MemoryLimit max memory hits (default 6).
	MemoryLimit int
}

func (o Options) autoIndex() bool {
	if o.AutoIndex == nil {
		return true
	}
	return *o.AutoIndex
}

func (o Options) writeFiles() bool {
	if o.WriteFiles == nil {
		return true
	}
	return *o.WriteFiles
}

func (o Options) packBudget() int {
	if o.PackBudget <= 0 {
		return DefaultPackBudget
	}
	return o.PackBudget
}

func (o Options) memoryLimit() int {
	if o.MemoryLimit <= 0 {
		return 6
	}
	return o.MemoryLimit
}

// Status is a quick readiness report for UI/CLI.
type Status struct {
	Cwd           string         `json:"cwd"`
	CwdAbs        string         `json:"cwd_abs"`
	Ready         bool           `json:"ready"`
	Map           projmap.Status `json:"map"`
	MemoryDocs    int            `json:"memory_docs"`
	MemoryEngine  string         `json:"memory_engine,omitempty"`
	HasContext    bool           `json:"has_context"`
	HasInstr      bool           `json:"has_instructions"`
	ContextPath   string         `json:"context_path,omitempty"`
	MapPath       string         `json:"map_path,omitempty"`
	Hints         []string       `json:"hints,omitempty"`
	Protocol      string         `json:"protocol,omitempty"`
}

// EnsureResult is returned after ensuring orientation.
type EnsureResult struct {
	Status        Status `json:"status"`
	MapGenerated  bool   `json:"map_generated"`
	MemoryIndexed int    `json:"memory_indexed,omitempty"`
	ContextWrote  bool   `json:"context_wrote"`
	PackChars     int    `json:"pack_chars,omitempty"`
	Seed          string `json:"seed,omitempty"` // short TTY seed protocol
}

// PackResult is a budgeted orientation pack.
type PackResult struct {
	Cwd       string `json:"cwd"`
	Markdown  string `json:"markdown"`
	Chars     int    `json:"chars"`
	Budget    int    `json:"budget"`
	WroteFile bool   `json:"wrote_file"`
	Path      string `json:"path,omitempty"`
	MapFresh  bool   `json:"map_fresh"`
	Hits      int    `json:"memory_hits,omitempty"`
}

// Manager ties map + memory for one agentsd process.
type Manager struct {
	Mem *memory.Store // optional
}

// New returns a context manager (mem may be nil).
func New(mem *memory.Store) *Manager {
	return &Manager{Mem: mem}
}

// ReadStatus reports orientation readiness without mutating the workspace.
func (m *Manager) ReadStatus(absRoot, relCwd string) Status {
	st := Status{
		Cwd:         relCwd,
		CwdAbs:      absRoot,
		Map:         projmap.ReadStatus(absRoot),
		MapPath:     filepath.Join(projmap.DirName, projmap.MapMarkdown),
		ContextPath: filepath.Join(projmap.DirName, ContextFile),
		Protocol:    ShortProtocol(relCwd),
	}
	if _, err := os.Stat(filepath.Join(absRoot, projmap.DirName, ContextFile)); err == nil {
		st.HasContext = true
	}
	if _, err := os.Stat(filepath.Join(absRoot, projmap.DirName, InstructionsFile)); err == nil {
		st.HasInstr = true
	}
	if m != nil && m.Mem != nil {
		n, _ := m.Mem.Stats(relCwd)
		st.MemoryDocs = n
		st.MemoryEngine = "sqlite-fts5"
		if m.Mem.EmbedderConfigured() {
			st.MemoryEngine = "sqlite-fts5+vector"
		}
	}
	st.Ready = st.Map.Exists && !st.Map.Stale
	if !st.Map.Exists {
		st.Hints = append(st.Hints, "no project map — run ensure or agentsctl map generate")
	} else if st.Map.Stale {
		st.Hints = append(st.Hints, "map stale: "+st.Map.Reason)
	}
	if m != nil && m.Mem != nil && st.MemoryDocs == 0 {
		st.Hints = append(st.Hints, "memory empty — ensure will index docs")
	}
	if !st.HasContext {
		st.Hints = append(st.Hints, "no CONTEXT.md pack yet")
	}
	return st
}

// Ensure refreshes map/memory/files so agents can orient quickly.
func (m *Manager) Ensure(absRoot, relCwd string, opt Options) (EnsureResult, error) {
	var out EnsureResult
	mapSt := projmap.ReadStatus(absRoot)
	needMap := opt.ForceMap || !mapSt.Exists || mapSt.Stale
	if needMap {
		if _, _, err := projmap.GenerateAndWrite(absRoot, relCwd); err != nil {
			return out, fmt.Errorf("map: %w", err)
		}
		out.MapGenerated = true
	}

	// memory index when empty / forced
	if m != nil && m.Mem != nil && (opt.ForceIndex || (opt.autoIndex() && mustIndex(m.Mem, relCwd, out.MapGenerated))) {
		n, err := m.Mem.IndexWorkspace(relCwd, absRoot, memory.IndexOptions{
			Clear:       opt.ForceIndex,
			IncludeCode: opt.IncludeCode,
		})
		if err != nil {
			// soft-fail index (map still useful)
			out.Status = m.ReadStatus(absRoot, relCwd)
			out.Status.Hints = append(out.Status.Hints, "memory index: "+err.Error())
		} else {
			out.MemoryIndexed = n
		}
	}

	pack, err := m.Pack(absRoot, relCwd, PackOptions{
		Budget:      opt.packBudget(),
		Query:       opt.Query,
		MemoryLimit: opt.memoryLimit(),
		WriteFile:   opt.writeFiles(),
	})
	if err != nil {
		return out, err
	}
	out.PackChars = pack.Chars
	out.ContextWrote = pack.WroteFile
	out.Seed = ComposeSeed(relCwd, "", DefaultSeedBudget)

	if opt.writeFiles() {
		_ = writeInstructions(absRoot, relCwd)
	}

	out.Status = m.ReadStatus(absRoot, relCwd)
	return out, nil
}

func mustIndex(mem *memory.Store, relCwd string, mapJustGenerated bool) bool {
	if mem == nil {
		return false
	}
	n, err := mem.Stats(relCwd)
	if err != nil {
		return true
	}
	if n == 0 {
		return true
	}
	// reindex lightly after map regen so PROJECT_MAP is fresh in FTS
	return mapJustGenerated
}

// PackOptions for Pack.
type PackOptions struct {
	Budget      int
	Query       string
	MemoryLimit int
	WriteFile   bool
}

// Pack builds a budgeted markdown orientation blob.
func (m *Manager) Pack(absRoot, relCwd string, opt PackOptions) (PackResult, error) {
	budget := opt.Budget
	if budget <= 0 {
		budget = DefaultPackBudget
	}
	limit := opt.MemoryLimit
	if limit <= 0 {
		limit = 6
	}

	var b strings.Builder
	b.WriteString("# Agent context pack\n\n")
	b.WriteString("_Generated by agentsd context manager · ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("_\n\n")
	b.WriteString("## Protocol\n\n")
	b.WriteString(ShortProtocol(relCwd))
	b.WriteString("\n")

	mapSt := projmap.ReadStatus(absRoot)
	mapFresh := mapSt.Exists && !mapSt.Stale
	if md, err := projmap.ReadMarkdown(absRoot); err == nil && strings.TrimSpace(md) != "" {
		b.WriteString("## Project map\n\n")
		b.WriteString(truncate(md, budget/2))
		b.WriteString("\n\n")
	} else {
		b.WriteString("## Project map\n\n_Missing — run `agentsctl context ensure` or `agentsctl map generate`._\n\n")
	}

	// Standing project instructions agents already know
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CODEX.md", "README.md", "README"} {
		rel := name
		abs := filepath.Join(absRoot, name)
		raw, err := os.ReadFile(abs)
		if err != nil || len(raw) == 0 {
			continue
		}
		text := redact.Line(string(raw))
		secBudget := budget / 6
		if secBudget < 800 {
			secBudget = 800
		}
		b.WriteString("## ")
		b.WriteString(rel)
		b.WriteString("\n\n")
		b.WriteString(truncate(text, secBudget))
		b.WriteString("\n\n")
		if b.Len() > budget*3/4 {
			break
		}
	}

	hitsN := 0
	if m != nil && m.Mem != nil && strings.TrimSpace(opt.Query) != "" {
		hits, err := m.Mem.SearchMode(relCwd, opt.Query, limit, memory.SearchAuto)
		if err == nil && len(hits) > 0 {
			b.WriteString("## Memory hits for `")
			b.WriteString(strings.ReplaceAll(opt.Query, "`", "'"))
			b.WriteString("`\n\n")
			for i, h := range hits {
				hitsN++
				fmt.Fprintf(&b, "%d. **%s** (`%s`)\n", i+1, empty(h.Title, h.Path), h.Path)
				if sn := strings.TrimSpace(h.Snippet); sn != "" {
					b.WriteString("   > ")
					b.WriteString(strings.ReplaceAll(truncate(sn, 280), "\n", " "))
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}
	}

	md := truncate(b.String(), budget)
	out := PackResult{
		Cwd:      relCwd,
		Markdown: md,
		Chars:    len(md),
		Budget:   budget,
		MapFresh: mapFresh,
		Hits:     hitsN,
		Path:     filepath.Join(projmap.DirName, ContextFile),
	}
	if opt.WriteFile {
		if err := writeContextFile(absRoot, md); err != nil {
			return out, err
		}
		out.WroteFile = true
	}
	return out, nil
}

// Note stores a durable free-form note in workspace memory (source=note).
func (m *Manager) Note(relCwd, title, text string) (memory.Doc, error) {
	if m == nil || m.Mem == nil {
		return memory.Doc{}, fmt.Errorf("memory disabled")
	}
	text = strings.TrimSpace(redact.Line(text))
	if text == "" {
		return memory.Doc{}, fmt.Errorf("empty note")
	}
	if title == "" {
		title = "note"
	}
	// path is stable-ish for delete/replace by title
	path := filepath.ToSlash(filepath.Join(projmap.DirName, "notes", sanitizeName(title)+".md"))
	return m.Mem.Upsert(memory.Doc{
		Workspace: relCwd,
		Path:      path,
		Title:     title,
		Source:    "note",
		Text:      text,
	})
}

// ShortProtocol is the standing orientation block for seeds and packs.
func ShortProtocol(relCwd string) string {
	cwd := relCwd
	if cwd == "" {
		cwd = "."
	}
	return fmt.Sprintf(
		"1. Read `.agents/PROJECT_MAP.md` (layout / stack / read-first).\n"+
			"2. Read `.agents/CONTEXT.md` if present (packed orientation).\n"+
			"3. Prefer targeted reads over full-tree walks.\n"+
			"4. Topic search: `agentsctl memory search -r %s \"…\"`.\n"+
			"5. After structural changes: `agentsctl context ensure -r %s`.\n",
		cwd, cwd,
	)
}

// ComposeSeed builds a short TTY seed: protocol pointer + optional user prompt.
// Keeps under budget so slow agent UIs still receive it via send-keys.
func ComposeSeed(relCwd, userPrompt string, budget int) string {
	if budget <= 0 {
		budget = DefaultSeedBudget
	}
	var b strings.Builder
	b.WriteString("[agentsd context] Orient before exploring:\n")
	b.WriteString("- Read .agents/PROJECT_MAP.md and .agents/CONTEXT.md\n")
	b.WriteString("- Search: agentsctl memory search -r ")
	if relCwd == "" {
		relCwd = "."
	}
	b.WriteString(relCwd)
	b.WriteString(" \"…\"\n")
	b.WriteString("- Standing rules: .agents/INSTRUCTIONS.md\n")
	userPrompt = strings.TrimSpace(userPrompt)
	if userPrompt != "" {
		b.WriteString("\n---\n")
		b.WriteString(userPrompt)
	}
	return truncate(b.String(), budget)
}

// MergeSeed prepends orientation seed to a user prompt (if seed enabled).
func MergeSeed(relCwd, userPrompt string, seed bool, budget int) string {
	if !seed {
		return userPrompt
	}
	return ComposeSeed(relCwd, userPrompt, budget)
}

func writeContextFile(absRoot, md string) error {
	dir := filepath.Join(absRoot, projmap.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ContextFile), []byte(md), 0o644)
}

func writeInstructions(absRoot, relCwd string) error {
	dir := filepath.Join(absRoot, projmap.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Agent instructions (agentsd)\n\n")
	b.WriteString(ShortProtocol(relCwd))
	b.WriteString("\n## Project map\n")
	b.WriteString("Always start from `.agents/PROJECT_MAP.md`. Regenerate via `agentsctl context ensure` or `agentsctl map generate`.\n\n")
	b.WriteString("## Memory\n")
	b.WriteString("Use `agentsctl memory search` for docs/notes. Add durable notes with `agentsctl context note`.\n\n")
	b.WriteString("## Do not\n")
	b.WriteString("- Dump entire monorepos into context when a map exists\n")
	b.WriteString("- Ignore AGENTS.md / CLAUDE.md listed under Read these first\n")
	return os.WriteFile(filepath.Join(dir, InstructionsFile), []byte(b.String()), 0o644)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	// try cut on paragraph
	cut := max - 20
	if cut < 1 {
		cut = max
	}
	if i := strings.LastIndex(s[:cut], "\n\n"); i > max/2 {
		return strings.TrimSpace(s[:i]) + "\n\n_…truncated…_"
	}
	if i := strings.LastIndex(s[:cut], "\n"); i > max/2 {
		return strings.TrimSpace(s[:i]) + "\n\n_…truncated…_"
	}
	return s[:cut] + "…\n\n_…truncated…_"
}

func empty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "note"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}
