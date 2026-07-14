package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/agentsinfo"
	"github.com/reloadlife/agents/internal/auth"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/job"
	"github.com/reloadlife/agents/internal/memory"
	"github.com/reloadlife/agents/internal/pathallow"
	"github.com/reloadlife/agents/internal/playwrightctl"
	"github.com/reloadlife/agents/internal/projmap"
	"github.com/reloadlife/agents/internal/session"
	"github.com/reloadlife/agents/internal/ghauth"
	"github.com/reloadlife/agents/internal/sshkeys"
	"github.com/reloadlife/agents/internal/status"
	"github.com/reloadlife/agents/internal/webui"
	"github.com/reloadlife/agents/internal/workspaces"
)

type Server struct {
	cfg  *config.Config
	mgr  *job.Manager
	sess *session.Manager
	mem  *memory.Store
	pw   *playwrightctl.Manager
	log  *slog.Logger
}

func New(cfg *config.Config, mgr *job.Manager, sess *session.Manager, mem *memory.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, mgr: mgr, sess: sess, mem: mem, pw: playwrightctl.New(cfg), log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/agents", s.handleListAgents)
	mux.HandleFunc("GET /v1/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /v1/workspaces/clone", s.handleCloneWorkspace)
	mux.HandleFunc("GET /v1/version", s.handleVersion)

	// SSH identity keys (public only; private never served)
	mux.HandleFunc("GET /v1/ssh-keys", s.handleListSSHKeys)
	mux.HandleFunc("POST /v1/ssh-keys", s.handleGenerateSSHKey)
	mux.HandleFunc("GET /v1/ssh-keys/{name}", s.handleGetSSHKey)
	mux.HandleFunc("DELETE /v1/ssh-keys/{name}", s.handleDeleteSSHKey)
	mux.HandleFunc("POST /v1/ssh-keys/{name}/delete", s.handleDeleteSSHKey)

	// GitHub CLI accounts (tokens never returned)
	mux.HandleFunc("GET /v1/gh/accounts", s.handleGHStatus)
	mux.HandleFunc("POST /v1/gh/login", s.handleGHLogin)
	mux.HandleFunc("POST /v1/gh/switch", s.handleGHSwitch)
	mux.HandleFunc("POST /v1/gh/logout", s.handleGHLogout)
	mux.HandleFunc("POST /v1/gh/setup-git", s.handleGHSetupGit)

	// Playwright / headed browser stack
	mux.HandleFunc("GET /v1/playwright", s.handlePlaywrightStatus)
	mux.HandleFunc("POST /v1/playwright/start", s.handlePlaywrightStart)
	mux.HandleFunc("POST /v1/playwright/stop", s.handlePlaywrightStop)
	mux.HandleFunc("POST /v1/playwright/restart", s.handlePlaywrightRestart)
	mux.HandleFunc("POST /v1/playwright/install", s.handlePlaywrightInstall)

	// Interactive TTY sessions (primary — not print/-p)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions/{id}/kill", s.handleKillSession)
	mux.HandleFunc("POST /v1/sessions/{id}/resume", s.handleResumeSession)
	mux.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /v1/sessions/{id}/delete", s.handleDeleteSession) // alias for clients without DELETE
	mux.HandleFunc("POST /v1/sessions/prune", s.handlePruneSessions)
	mux.HandleFunc("GET /v1/sessions/{id}/history", s.handleSessionHistory)
	// Full remote PTY (tmux attach) over WebSocket — no SSH required
	mux.HandleFunc("GET /v1/sessions/{id}/pty", s.handleSessionPTY)

	// Project maps (durable orientation files under <cwd>/.agents/)
	mux.HandleFunc("POST /v1/maps", s.handleGenerateMap)
	mux.HandleFunc("GET /v1/maps", s.handleGetMap)
	mux.HandleFunc("GET /v1/maps/status", s.handleMapStatus)

	// Workspace memory (FTS) — agents query via CLI/API
	mux.HandleFunc("POST /v1/memory/index", s.handleMemoryIndex)
	mux.HandleFunc("POST /v1/memory/search", s.handleMemorySearch)
	mux.HandleFunc("GET /v1/memory/stats", s.handleMemoryStats)

	// Print/API jobs (secondary — uses credits; explicit only)
	mux.HandleFunc("POST /v1/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /v1/jobs", s.handleListJobs)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /v1/jobs/{id}/log", s.handleGetLog)
	mux.HandleFunc("GET /v1/jobs/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.handleCancel)
	mux.HandleFunc("POST /v1/jobs/{id}/confirm", s.handleConfirm)

	// Embedded browser UI (static SPA). More-specific /v1 routes take precedence.
	if s.cfg.WebEnabled() {
		mux.Handle("/", webui.Handler())
	}

	return auth.Middleware(s.cfg.Token, withLogging(s.log, mux))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "agentsd"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	running, queued := s.mgr.Stats()
	snap := status.Collect(r.Context(), s.cfg, running, queued)
	// attach session count
	type withSessions struct {
		status.Snapshot
		Sessions int               `json:"tty_sessions"`
		Agents   []agentsinfo.Info `json:"agents,omitempty"`
	}
	out := withSessions{Snapshot: snap, Agents: agentsinfo.List(s.cfg)}
	if s.sess != nil {
		out.Sessions = s.sess.RunningCount()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	list := agentsinfo.List(s.cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"agents":    list,
		"available": agentsinfo.AvailableTTY(s.cfg),
	})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	list := workspaces.List(s.cfg)
	// Also expose flat path strings for simple clients.
	paths := make([]string, 0, len(list))
	for _, e := range list {
		paths = append(paths, e.Path)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_root": s.cfg.WorkspaceRoot,
		"default_cwd":    s.cfg.DefaultCwd,
		"workspaces":     list,
		"paths":          paths,
	})
}

func (s *Server) handleCloneWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspaces.CloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := workspaces.Clone(s.cfg, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) sshKeys() (*sshkeys.Manager, error) {
	return sshkeys.New("") // agentsd user's ~/.ssh
}

func (s *Server) handleListSSHKeys(w http.ResponseWriter, r *http.Request) {
	m, err := s.sshKeys()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	list, err := m.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dir":  m.Dir(),
		"keys": list,
	})
}

func (s *Server) handleGetSSHKey(w http.ResponseWriter, r *http.Request) {
	m, err := s.sshKeys()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	k, err := m.Get(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !k.HasPrivate && !k.HasPublic {
		writeErr(w, http.StatusNotFound, fmt.Errorf("key not found"))
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleGenerateSSHKey(w http.ResponseWriter, r *http.Request) {
	m, err := s.sshKeys()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var req sshkeys.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	k, err := m.Generate(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.log.Info("ssh key generated", "name", k.Name, "type", k.Type)
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) handleDeleteSSHKey(w http.ResponseWriter, r *http.Request) {
	m, err := s.sshKeys()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	name := r.PathValue("name")
	if err := m.Delete(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.log.Info("ssh key deleted", "name", name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "deleted": true})
}

func (s *Server) ghMgr() (*ghauth.Manager, error) {
	return ghauth.New()
}

func (s *Server) handleGHStatus(w http.ResponseWriter, r *http.Request) {
	m, err := s.ghMgr()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	st, err := m.Status()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGHLogin(w http.ResponseWriter, r *http.Request) {
	m, err := s.ghMgr()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	var req ghauth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// never log the token
	st, err := m.Login(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.log.Info("gh login", "host", req.Host, "accounts", len(st.Accounts), "active", st.Active)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGHSwitch(w http.ResponseWriter, r *http.Request) {
	m, err := s.ghMgr()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	var req ghauth.SwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	st, err := m.Switch(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.log.Info("gh switch", "user", req.User, "host", req.Host, "active", st.Active)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGHLogout(w http.ResponseWriter, r *http.Request) {
	m, err := s.ghMgr()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	var req ghauth.LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	st, err := m.Logout(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.log.Info("gh logout", "user", req.User, "host", req.Host)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGHSetupGit(w http.ResponseWriter, r *http.Request) {
	m, err := s.ghMgr()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	if err := m.SetupGit(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	st, _ := m.Status()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": st})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "agentsd",
		"api":     "v1",
	})
}

func (s *Server) handlePlaywrightStatus(w http.ResponseWriter, r *http.Request) {
	if s.pw == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("playwright manager unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, s.pw.Status())
}

func (s *Server) handlePlaywrightStart(w http.ResponseWriter, r *http.Request) {
	if s.pw == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("playwright manager unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	st, err := s.pw.Start(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "status": st})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": st})
}

func (s *Server) handlePlaywrightStop(w http.ResponseWriter, r *http.Request) {
	if s.pw == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("playwright manager unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	st, err := s.pw.Stop(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "status": st})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": st})
}

func (s *Server) handlePlaywrightRestart(w http.ResponseWriter, r *http.Request) {
	if s.pw == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("playwright manager unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	st, err := s.pw.Restart(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "status": st})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": st})
}

func (s *Server) handlePlaywrightInstall(w http.ResponseWriter, r *http.Request) {
	if s.pw == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("playwright manager unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	out, err := s.pw.InstallBrowsers(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "output": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "status": s.pw.Status()})
}

func (s *Server) handlePruneSessions(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	var body struct {
		MaxAge string `json:"max_age"` // e.g. "24h"; empty = all non-running
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var maxAge time.Duration
	if body.MaxAge != "" {
		d, err := time.ParseDuration(body.MaxAge)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("max_age: %w", err))
			return
		}
		maxAge = d
	}
	n, err := s.sess.Prune(maxAge)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	var req session.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Mode == session.ModePrint {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("use POST /v1/jobs for print/API mode; sessions are TTY-only"))
		return
	}
	req.Mode = session.ModeTTY
	sess, err := s.sess.Create(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	list, err := s.sess.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	sess, err := s.sess.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	sess, err := s.sess.Kill(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	id := r.PathValue("id")
	if err := s.sess.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "deleted": true})
}

func (s *Server) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	sess, err := s.sess.Resume(r.PathValue("id"))
	if err != nil {
		// not found vs config/cwd failures
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	data, source, err := s.sess.History(r.PathValue("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Plain terminal dump (may include ANSI). Optional JSON via ?format=json
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]any{
			"source": source,
			"bytes":  len(data),
			"text":   string(data),
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Agents-History-Source", source)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleSessionPTY(w http.ResponseWriter, r *http.Request) {
	if s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("sessions not enabled"))
		return
	}
	// WebSocket hijack — long lived; do not use JSON helpers
	s.sess.HandlePTY(w, r, r.PathValue("id"))
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req job.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	j, err := s.mgr.Create(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	resp := map[string]any{
		"id":         j.ID,
		"state":      j.State,
		"stream_url": "/v1/jobs/" + j.ID + "/events",
		"warning":    "print/API job mode — may use API credits. Prefer: agentsctl session start -a claude",
	}
	if j.State == job.StateAwaitingConfirm {
		resp["confirm_token"] = j.Confirm
		resp["message"] = "elevated caps require confirm"
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	list, err := s.mgr.List(50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for _, j := range list {
		j.Confirm = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, err := s.mgr.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	j.Confirm = ""
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := s.mgr.ReadLog(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, err := s.mgr.Cancel(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	j.Confirm = ""
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	j, err := s.mgr.Confirm(id, body.Token)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	j.Confirm = ""
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, err := s.mgr.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if b, err := s.mgr.ReadLog(id); err == nil && len(b) > 0 {
		for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			writeSSE(w, "log", map[string]string{"line": line})
		}
		flusher.Flush()
	}
	writeSSE(w, "state", map[string]string{"state": string(j.State)})
	flusher.Flush()

	if j.State.Terminal() {
		writeSSE(w, "result", map[string]any{
			"exit_code": j.ExitCode,
			"summary":   j.Summary,
			"error":     j.Error,
			"state":     j.State,
		})
		flusher.Flush()
		return
	}

	ch, unsub := s.mgr.SubscribeLog(id)
	defer unsub()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, "log", map[string]string{"line": line})
			flusher.Flush()
		case <-ticker.C:
			jj, err := s.mgr.Get(id)
			if err != nil {
				return
			}
			writeSSE(w, "state", map[string]string{"state": string(jj.State)})
			flusher.Flush()
			if jj.State.Terminal() {
				writeSSE(w, "result", map[string]any{
					"exit_code": jj.ExitCode,
					"summary":   jj.Summary,
					"error":     jj.Error,
					"state":     jj.State,
				})
				flusher.Flush()
				return
			}
		}
	}
}

func (s *Server) resolveMapCwd(rel string) (abs string, relOut string, err error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		if s.cfg.DefaultCwd != "" {
			rel = s.cfg.DefaultCwd
		} else {
			rel = "."
		}
	}
	abs, err = pathallow.Resolve(s.cfg.WorkspaceRoot, rel, s.cfg.Allow.Paths)
	if err != nil {
		return "", "", err
	}
	return abs, rel, nil
}

func (s *Server) handleGenerateMap(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cwd string `json:"cwd"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	abs, rel, err := s.resolveMapCwd(body.Cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	m, metaPath, err := projmap.GenerateAndWrite(abs, rel)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"cwd":       rel,
		"cwd_abs":   abs,
		"map_path":  filepath.Join(abs, projmap.DirName, projmap.MapMarkdown),
		"meta_path": metaPath,
		"map":       m,
		"status":    projmap.ReadStatus(abs),
	})
}

func (s *Server) handleGetMap(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	abs, rel, err := s.resolveMapCwd(cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "markdown"
	}
	st := projmap.ReadStatus(abs)
	if !st.Exists {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":  "no project map — run agentsctl map generate",
			"status": st,
			"cwd":    rel,
		})
		return
	}
	switch format {
	case "status":
		writeJSON(w, http.StatusOK, map[string]any{"cwd": rel, "cwd_abs": abs, "status": st})
	case "json":
		// re-read structured file
		b, err := os.ReadFile(filepath.Join(abs, projmap.DirName, projmap.MapJSON))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	default:
		md, err := projmap.ReadMarkdown(abs)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if r.URL.Query().Get("raw") == "1" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(md))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"cwd":      rel,
			"cwd_abs":  abs,
			"status":   st,
			"markdown": md,
		})
	}
}

func (s *Server) handleMapStatus(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	abs, rel, err := s.resolveMapCwd(cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":     rel,
		"cwd_abs": abs,
		"status":  projmap.ReadStatus(abs),
	})
}

func (s *Server) handleMemoryIndex(w http.ResponseWriter, r *http.Request) {
	if s.mem == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("memory disabled"))
		return
	}
	var body struct {
		Cwd         string `json:"cwd"`
		Clear       bool   `json:"clear"`
		IncludeCode bool   `json:"include_code"`
		// Also ensure project map exists before index
		GenerateMap bool `json:"generate_map"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	abs, rel, err := s.resolveMapCwd(body.Cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.GenerateMap {
		if _, _, err := projmap.GenerateAndWrite(abs, rel); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("map: %w", err))
			return
		}
	}
	n, err := s.mem.IndexWorkspace(rel, abs, memory.IndexOptions{
		Clear:       body.Clear,
		IncludeCode: body.IncludeCode,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	count, _ := s.mem.Stats(rel)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"cwd":       rel,
		"indexed":   n,
		"docs_total": count,
	})
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if s.mem == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("memory disabled"))
		return
	}
	var body struct {
		Cwd   string `json:"cwd"`
		Query string `json:"query"`
		Limit int    `json:"limit"`
		Mode  string `json:"mode"` // auto|fts|vector
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_, rel, err := s.resolveMapCwd(body.Cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	mode := memory.SearchMode(body.Mode)
	if mode == "" {
		mode = memory.SearchAuto
	}
	hits, err := s.mem.SearchMode(rel, body.Query, body.Limit, mode)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":    rel,
		"query":  body.Query,
		"mode":   mode,
		"hits":   hits,
		"vector": s.mem.EmbedderConfigured(),
	})
}

func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if s.mem == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("memory disabled"))
		return
	}
	cwd := r.URL.Query().Get("cwd")
	ws := ""
	if cwd != "" {
		_, rel, err := s.resolveMapCwd(cwd)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws = rel
	}
	count, err := s.mem.Stats(ws)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	withEmb, _ := s.mem.VectorStats(ws)
	engine := "sqlite-fts5"
	if s.mem.EmbedderConfigured() {
		engine = "sqlite-fts5+vector"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":            ws,
		"docs":           count,
		"docs_embedded":  withEmb,
		"dir":            s.cfg.MemoryDir(),
		"engine":         engine,
		"embed_model":    s.mem.EmbedModel(),
		"vector_enabled": s.mem.EmbedderConfigured(),
	})
}

func writeSSE(w http.ResponseWriter, event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
	})
}
