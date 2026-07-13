package job

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists job metadata in sqlite and logs on disk.
type Store struct {
	dir string
	db  *sql.DB
	mu  sync.Mutex
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dir, "jobs.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  json TEXT NOT NULL,
  state TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);
`)
	return err
}

func (s *Store) Dir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *Store) LogPath(id string) string {
	return filepath.Join(s.Dir(id), "log.txt")
}

func (s *Store) Put(j *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir(j.ID), 0o700); err != nil {
		return err
	}
	meta := filepath.Join(s.Dir(j.ID), "meta.json")
	if err := os.WriteFile(meta, b, 0o600); err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO jobs(id, json, state, created_at) VALUES(?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET json=excluded.json, state=excluded.state`,
		j.ID, string(b), string(j.State), j.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) Get(id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	err := s.db.QueryRow(`SELECT json FROM jobs WHERE id=?`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) List(limit int) ([]*Job, error) {
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT json FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var j Job
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			return nil, err
		}
		out = append(out, &j)
	}
	return out, rows.Err()
}

// MarkInterrupted sets any non-terminal running/queued-in-memory recovery:
// on daemon start, running jobs become interrupted.
func (s *Store) MarkInterrupted() error {
	jobs, err := s.List(1000)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, j := range jobs {
		if j.State == StateRunning {
			j.State = StateInterrupted
			j.EndedAt = &now
			j.Error = "daemon restarted while job was running"
			if err := s.Put(j); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) WriteResult(id string, r Result) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir(id), "result.json"), b, 0o600)
}

func (s *Store) AppendLog(id, line string) error {
	f, err := os.OpenFile(s.LogPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	if len(line) == 0 || line[len(line)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}
	return err
}

func (s *Store) ReadLog(id string) ([]byte, error) {
	b, err := os.ReadFile(s.LogPath(id))
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return b, err
}
