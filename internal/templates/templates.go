// Package templates stores reusable session start presets.
package templates

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
)

// Template is a saved session launch profile.
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Agent       string    `json:"agent"`
	Cwd         string    `json:"cwd"`
	Prompt      string    `json:"prompt,omitempty"`
	Account     string    `json:"account,omitempty"`
	AccountMode string    `json:"account_mode,omitempty"`
	EnsureCtx   bool      `json:"ensure_context,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func New(jobsDir string) (*Store, error) {
	dir := filepath.Join(jobsDir, "templates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "templates.json")}, nil
}

func (s *Store) load() ([]Template, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []Template
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) save(list []Template) error {
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *Store) List() ([]Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	return list, nil
}

func (s *Store) Get(id string) (*Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			t := list[i]
			return &t, nil
		}
	}
	return nil, fmt.Errorf("template not found")
}

type UpsertRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Agent       string `json:"agent"`
	Cwd         string `json:"cwd"`
	Prompt      string `json:"prompt,omitempty"`
	Account     string `json:"account,omitempty"`
	AccountMode string `json:"account_mode,omitempty"`
	EnsureCtx   bool   `json:"ensure_context"`
}

func (s *Store) Upsert(req UpsertRequest) (*Template, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Agent = strings.TrimSpace(req.Agent)
	req.Cwd = strings.TrimSpace(req.Cwd)
	if req.Name == "" || req.Agent == "" || req.Cwd == "" {
		return nil, fmt.Errorf("name, agent, and cwd are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if req.ID != "" {
		for i := range list {
			if list[i].ID == req.ID {
				list[i].Name = req.Name
				list[i].Agent = req.Agent
				list[i].Cwd = req.Cwd
				list[i].Prompt = req.Prompt
				list[i].Account = req.Account
				list[i].AccountMode = req.AccountMode
				list[i].EnsureCtx = req.EnsureCtx
				list[i].UpdatedAt = now
				if err := s.save(list); err != nil {
					return nil, err
				}
				t := list[i]
				return &t, nil
			}
		}
	}
	t := Template{
		ID:          "t_" + ulid.Make().String(),
		Name:        req.Name,
		Agent:       req.Agent,
		Cwd:         req.Cwd,
		Prompt:      req.Prompt,
		Account:     req.Account,
		AccountMode: req.AccountMode,
		EnsureCtx:   req.EnsureCtx,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	list = append(list, t)
	if err := s.save(list); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return err
	}
	out := list[:0]
	found := false
	for _, t := range list {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("template not found")
	}
	return s.save(out)
}
