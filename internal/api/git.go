package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/reloadlife/agents/internal/workspaces"
)

// GET /v1/git/status?cwd=<rel>
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	out, err := workspaces.Status(s.cfg, cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /v1/git/diff?cwd=<rel>&path=<optional>&staged=0|1&base=<optional ref>
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cwd := q.Get("cwd")
	path := q.Get("path")
	base := q.Get("base")
	staged, err := parseBool01(q.Get("staged"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := workspaces.Diff(s.cfg, cwd, path, staged, base)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /v1/git/file?cwd=&path=&ref=  (blob at ref for side-by-side)
func (s *Server) handleGitFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cwd := q.Get("cwd")
	path := strings.TrimSpace(q.Get("path"))
	if path == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("path required"))
		return
	}
	ref := q.Get("ref")
	out, err := workspaces.FileAtRef(s.cfg, cwd, path, ref)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /v1/git/worktrees?cwd=<rel>
func (s *Server) handleGitWorktrees(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	if strings.TrimSpace(cwd) == "" {
		cwd = s.cfg.DefaultCwd
		if cwd == "" {
			cwd = "."
		}
	}
	list, err := workspaces.ListWorktrees(s.cfg.WorkspaceRoot, cwd, s.cfg.Allow.Paths)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":       cwd,
		"worktrees": list,
	})
}

// parseBool01 accepts "", "0","1","true","false","yes","no" (case-insensitive).
func parseBool01(v string) (bool, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "0" || v == "false" || v == "no" {
		return false, nil
	}
	if v == "1" || v == "true" || v == "yes" {
		return true, nil
	}
	// also accept strconv for completeness
	if b, err := strconv.ParseBool(v); err == nil {
		return b, nil
	}
	return false, fmt.Errorf("staged must be 0 or 1")
}
