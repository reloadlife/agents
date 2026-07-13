package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/agentsinfo"
	"github.com/reloadlife/agents/internal/auth"
	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/job"
	"github.com/reloadlife/agents/internal/session"
	"github.com/reloadlife/agents/internal/status"
	"github.com/reloadlife/agents/internal/workspaces"
)

type Server struct {
	cfg  *config.Config
	mgr  *job.Manager
	sess *session.Manager
	log  *slog.Logger
}

func New(cfg *config.Config, mgr *job.Manager, sess *session.Manager, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, mgr: mgr, sess: sess, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/agents", s.handleListAgents)
	mux.HandleFunc("GET /v1/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("GET /v1/version", s.handleVersion)

	// Interactive TTY sessions (primary — not print/-p)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions/{id}/kill", s.handleKillSession)
	mux.HandleFunc("POST /v1/sessions/prune", s.handlePruneSessions)
	// Full remote PTY (tmux attach) over WebSocket — no SSH required
	mux.HandleFunc("GET /v1/sessions/{id}/pty", s.handleSessionPTY)

	// Print/API jobs (secondary — uses credits; explicit only)
	mux.HandleFunc("POST /v1/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /v1/jobs", s.handleListJobs)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /v1/jobs/{id}/log", s.handleGetLog)
	mux.HandleFunc("GET /v1/jobs/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.handleCancel)
	mux.HandleFunc("POST /v1/jobs/{id}/confirm", s.handleConfirm)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_root": s.cfg.WorkspaceRoot,
		"default_cwd":    s.cfg.DefaultCwd,
		"workspaces":     workspaces.List(s.cfg),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "agentsd",
		"api":     "v1",
	})
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
