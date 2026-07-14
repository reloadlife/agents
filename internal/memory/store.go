// Package memory provides workspace-scoped text memory (SQLite FTS5 + optional vectors).
// Agents query via CLI/API; agentsd does not inject into third-party LLM loops.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

// Store is an FTS + optional vector memory under a data directory.
type Store struct {
	dir      string
	db       *sql.DB
	mu       sync.Mutex
	embedder Embedder
}

// Doc is one indexed document/chunk.
type Doc struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Path      string    `json:"path,omitempty"`
	Title     string    `json:"title,omitempty"`
	Source    string    `json:"source,omitempty"` // map|readme|file|note
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Embedding []float32 `json:"-"`
}

// Hit is a search result.
type Hit struct {
	ID        string  `json:"id"`
	Workspace string  `json:"workspace"`
	Path      string  `json:"path,omitempty"`
	Title     string  `json:"title,omitempty"`
	Source    string  `json:"source,omitempty"`
	Snippet   string  `json:"snippet"`
	Rank      float64 `json:"rank"`
	Mode      string  `json:"mode,omitempty"` // fts|vector
}

// OpenOptions configures the memory store.
type OpenOptions struct {
	Embedder Embedder
}

// Open opens or creates the memory database under dir.
func Open(dir string) (*Store, error) {
	return OpenWith(dir, OpenOptions{})
}

// OpenWith opens the store with optional embedder.
func OpenWith(dir string, opt OpenOptions) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dir, "memory.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, db: db, embedder: opt.Embedder}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// SetEmbedder replaces the embedder (may be nil → FTS only).
func (s *Store) SetEmbedder(e Embedder) { s.embedder = e }

// EmbedderConfigured reports whether vector embeddings are available.
func (s *Store) EmbedderConfigured() bool { return s != nil && s.embedder != nil }

// EmbedModel returns the configured model name, or empty.
func (s *Store) EmbedModel() string {
	if s == nil || s.embedder == nil {
		return ""
	}
	return s.embedder.Model()
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS docs (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL,
  path TEXT,
  title TEXT,
  source TEXT,
  text TEXT NOT NULL,
  created_at TEXT NOT NULL,
  embedding BLOB
);
CREATE INDEX IF NOT EXISTS idx_docs_ws ON docs(workspace);

CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(
  text,
  title,
  path,
  content='docs',
  content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS docs_ai AFTER INSERT ON docs BEGIN
  INSERT INTO docs_fts(rowid, text, title, path) VALUES (new.rowid, new.text, new.title, new.path);
END;
CREATE TRIGGER IF NOT EXISTS docs_ad AFTER DELETE ON docs BEGIN
  INSERT INTO docs_fts(docs_fts, rowid, text, title, path) VALUES('delete', old.rowid, old.text, old.title, old.path);
END;
CREATE TRIGGER IF NOT EXISTS docs_au AFTER UPDATE ON docs BEGIN
  INSERT INTO docs_fts(docs_fts, rowid, text, title, path) VALUES('delete', old.rowid, old.text, old.title, old.path);
  INSERT INTO docs_fts(rowid, text, title, path) VALUES (new.rowid, new.text, new.title, new.path);
END;
`)
	if err != nil {
		return err
	}
	// older DBs may lack embedding column
	_, _ = s.db.Exec(`ALTER TABLE docs ADD COLUMN embedding BLOB`)
	return nil
}

// Upsert inserts or replaces a document. Empty ID generates a new ULID.
// When an embedder is configured, computes and stores the vector.
func (s *Store) Upsert(d Doc) (Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Workspace == "" {
		return Doc{}, fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(d.Text) == "" {
		return Doc{}, fmt.Errorf("text required")
	}
	if d.ID == "" {
		d.ID = ulid.Make().String()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	if d.Path != "" {
		_, _ = s.db.Exec(`DELETE FROM docs WHERE workspace=? AND path=?`, d.Workspace, d.Path)
	}

	var embBlob []byte
	if s.embedder != nil && len(d.Embedding) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		vecs, err := s.embedder.Embed(ctx, []string{d.Text})
		cancel()
		if err == nil && len(vecs) > 0 {
			d.Embedding = vecs[0]
		}
		// embed failure: still store text for FTS
	}
	if len(d.Embedding) > 0 {
		embBlob = packFloat32(d.Embedding)
	}

	_, err := s.db.Exec(
		`INSERT INTO docs(id, workspace, path, title, source, text, created_at, embedding) VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET workspace=excluded.workspace, path=excluded.path,
		 title=excluded.title, source=excluded.source, text=excluded.text, created_at=excluded.created_at,
		 embedding=excluded.embedding`,
		d.ID, d.Workspace, d.Path, d.Title, d.Source, d.Text, d.CreatedAt.UTC().Format(time.RFC3339Nano), embBlob,
	)
	return d, err
}

// DeleteByPath removes docs for a workspace path.
func (s *Store) DeleteByPath(workspace, path string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM docs WHERE workspace=? AND path=?`, workspace, path)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deletePathPrefix removes path and path#chunk-* docs.
func (s *Store) deletePathPrefix(workspace, path string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`DELETE FROM docs WHERE workspace=? AND (path=? OR path LIKE ?)`,
		workspace, path, path+"#chunk-%",
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ClearWorkspace removes all docs for a workspace key.
func (s *Store) ClearWorkspace(workspace string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM docs WHERE workspace=?`, workspace)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SearchMode selects retrieval strategy.
type SearchMode string

const (
	SearchAuto   SearchMode = "auto"   // vector if available, else FTS
	SearchFTS    SearchMode = "fts"
	SearchVector SearchMode = "vector"
)

// Search runs retrieval within a workspace.
func (s *Store) Search(workspace, query string, limit int) ([]Hit, error) {
	return s.SearchMode(workspace, query, limit, SearchAuto)
}

// SearchMode runs FTS and/or vector search.
func (s *Store) SearchMode(workspace, query string, limit int, mode SearchMode) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	if mode == "" {
		mode = SearchAuto
	}

	useVector := (mode == SearchVector || mode == SearchAuto) && s.embedder != nil
	useFTS := mode == SearchFTS || mode == SearchAuto || mode == SearchVector

	// Hybrid (auto): merge FTS + vector via simple RRF when both available.
	if mode == SearchAuto && useVector && useFTS {
		ftsHits, ftsErr := s.searchFTS(workspace, query, limit)
		vecHits, vecErr := s.searchVector(workspace, query, limit)
		if ftsErr == nil && vecErr == nil && (len(ftsHits) > 0 || len(vecHits) > 0) {
			return mergeRRF(ftsHits, vecHits, limit), nil
		}
		if ftsErr == nil && len(ftsHits) > 0 {
			return ftsHits, nil
		}
		if vecErr == nil {
			return vecHits, vecErr
		}
	}

	if useVector {
		hits, err := s.searchVector(workspace, query, limit)
		if err == nil && len(hits) > 0 {
			return hits, nil
		}
		if mode == SearchVector {
			if err != nil {
				return nil, err
			}
			return hits, nil
		}
		// auto: fall through to FTS
	}
	if useFTS {
		return s.searchFTS(workspace, query, limit)
	}
	return nil, fmt.Errorf("no search backend")
}

// mergeRRF fuses two ranked lists (Reciprocal Rank Fusion).
func mergeRRF(a, b []Hit, limit int) []Hit {
	const k = 60.0
	type scored struct {
		h Hit
		s float64
	}
	byID := map[string]*scored{}
	add := func(list []Hit) {
		for i, h := range list {
			id := h.ID
			if id == "" {
				id = h.Path + h.Title
			}
			sc := 1.0 / (k + float64(i+1))
			if cur, ok := byID[id]; ok {
				cur.s += sc
				if h.Mode != "" && cur.h.Mode != "" && h.Mode != cur.h.Mode {
					cur.h.Mode = "hybrid"
				}
			} else {
				cp := h
				if cp.Mode == "" {
					cp.Mode = "fts"
				}
				byID[id] = &scored{h: cp, s: sc}
			}
		}
	}
	add(a)
	add(b)
	out := make([]scored, 0, len(byID))
	for _, v := range byID {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	hits := make([]Hit, len(out))
	for i := range out {
		hits[i] = out[i].h
		hits[i].Rank = out[i].s
		if hits[i].Mode != "hybrid" && len(a) > 0 && len(b) > 0 {
			// leave mode as-is when only one list contributed
		}
	}
	return hits
}

func (s *Store) searchFTS(workspace, query string, limit int) ([]Hit, error) {
	q := ftsQuery(query)
	if q == "" {
		return nil, fmt.Errorf("empty query")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
SELECT d.id, d.workspace, d.path, d.title, d.source,
       snippet(docs_fts, 0, '«', '»', '…', 12),
       bm25(docs_fts)
FROM docs_fts
JOIN docs d ON d.rowid = docs_fts.rowid
WHERE docs_fts MATCH ? AND d.workspace = ?
ORDER BY bm25(docs_fts)
LIMIT ?`, q, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Workspace, &h.Path, &h.Title, &h.Source, &h.Snippet, &h.Rank); err != nil {
			return nil, err
		}
		h.Mode = "fts"
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (s *Store) searchVector(workspace, query string, limit int) ([]Hit, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("embedder not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	qvec := vecs[0]

	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
SELECT id, workspace, path, title, source, text, embedding
FROM docs
WHERE workspace = ? AND embedding IS NOT NULL AND length(embedding) > 0`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		h Hit
		s float64
	}
	var all []scored
	for rows.Next() {
		var id, ws, path, title, source, text string
		var emb []byte
		if err := rows.Scan(&id, &ws, &path, &title, &source, &text, &emb); err != nil {
			return nil, err
		}
		v := unpackFloat32(emb)
		if len(v) == 0 {
			continue
		}
		sim := cosineSim(qvec, v)
		all = append(all, scored{
			h: Hit{
				ID: id, Workspace: ws, Path: path, Title: title, Source: source,
				Snippet: snippetFromText(text, 180),
				Rank:    -sim, // lower is better for consistency with bm25 ordering display
				Mode:    "vector",
			},
			s: sim,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
	if len(all) > limit {
		all = all[:limit]
	}
	hits := make([]Hit, len(all))
	for i := range all {
		hits[i] = all[i].h
		// expose similarity as positive rank for API consumers
		hits[i].Rank = all[i].s
	}
	return hits, nil
}

// Stats returns document counts per workspace (or one workspace).
func (s *Store) Stats(workspace string) (count int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if workspace == "" {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM docs`).Scan(&count)
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM docs WHERE workspace=?`, workspace).Scan(&count)
	return
}

// VectorStats returns how many docs have embeddings for a workspace.
func (s *Store) VectorStats(workspace string) (withEmb int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if workspace == "" {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM docs WHERE embedding IS NOT NULL AND length(embedding)>0`).Scan(&withEmb)
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM docs WHERE workspace=? AND embedding IS NOT NULL AND length(embedding)>0`, workspace).Scan(&withEmb)
	return
}

// ftsQuery turns free text into a safe FTS5 MATCH expression (AND of tokens).
func ftsQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	repl := strings.NewReplacer(`"`, " ", `'`, " ", `*`, " ", `(`, " ", `)`, " ", `:`, " ", `^`, " ")
	q = repl.Replace(q)
	parts := strings.Fields(q)
	var toks []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) < 2 {
			continue
		}
		toks = append(toks, `"`+p+`"`)
	}
	return strings.Join(toks, " ")
}
