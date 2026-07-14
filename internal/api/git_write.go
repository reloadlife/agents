package api

import (
	"encoding/json"
	"net/http"

	"github.com/reloadlife/agents/internal/workspaces"
)

// POST /v1/git/commit
// Body: { "cwd", "message", "paths"?, "all"? }
// Stages and commits locally. NEVER pushes.
func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	var req workspaces.CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := workspaces.Commit(s.cfg, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "git.commit", out.Cwd, map[string]any{
		"commit":  out.Commit,
		"branch":  out.Branch,
		"message": out.Message,
		"all":     req.All,
		"paths":   req.Paths,
	})
	writeJSON(w, http.StatusOK, out)
}

// POST /v1/git/pull-request
// Body: { "cwd", "title", "body"?, "base"?, "draft"? }
// Creates a PR via `gh pr create` in the resolved cwd (host must have gh auth).
func (s *Server) handleGitPullRequest(w http.ResponseWriter, r *http.Request) {
	var req workspaces.PullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := workspaces.CreatePullRequest(s.cfg, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "git.pull_request", out.Cwd, map[string]any{
		"url":    out.URL,
		"number": out.Number,
		"title":  req.Title,
		"base":   req.Base,
		"draft":  req.Draft,
	})
	writeJSON(w, http.StatusOK, out)
}
