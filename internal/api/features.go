package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/auth"
	"github.com/reloadlife/agents/internal/backup"
	"github.com/reloadlife/agents/internal/ctxmgr"
	"github.com/reloadlife/agents/internal/projmap"
	"github.com/reloadlife/agents/internal/recording"
	"github.com/reloadlife/agents/internal/session"
	"github.com/reloadlife/agents/internal/templates"
	"github.com/reloadlife/agents/internal/uploads"
	"github.com/reloadlife/agents/internal/workspaces"
)

func (s *Server) actor(r *http.Request) string {
	return auth.ActorFrom(r.Context())
}

func (s *Server) clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

func (s *Server) audit(r *http.Request, action, target string, detail map[string]any) {
	if s.auditLog == nil {
		return
	}
	s.auditLog.Record(action, s.actor(r), target, s.clientIP(r), detail)
}

// —— Recordings ——

func (s *Server) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	if s.rec == nil {
		writeJSON(w, http.StatusOK, map[string]any{"recordings": []any{}, "enabled": false})
		return
	}
	sid := r.URL.Query().Get("session_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var list []recording.Meta
	var err error
	if q != "" {
		list, err = s.rec.Search(q, limit)
	} else {
		list, err = s.rec.List(sid, limit)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recordings": list,
		"enabled":    s.cfg.RecordingEnabled(),
		"dir":        s.cfg.RecordingsDir(),
	})
}

func (s *Server) handleGetRecording(w http.ResponseWriter, r *http.Request) {
	if s.rec == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("recording disabled"))
		return
	}
	meta, pane, err := s.rec.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if r.URL.Query().Get("raw") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pane)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"meta": meta,
		"text": string(pane),
	})
}

func (s *Server) handleManualRecord(w http.ResponseWriter, r *http.Request) {
	if s.rec == nil || !s.cfg.RecordingEnabled() {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("recording disabled — set sessions.recording = true"))
		return
	}
	id := r.PathValue("id")
	s.sess.SnapshotHistoryReason(id, "manual")
	// re-list latest for this session
	list, _ := s.rec.List(id, 1)
	s.audit(r, "recording.manual", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latest": list})
}

// —— Templates ——

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	if s.tmpl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"templates": []any{}})
		return
	}
	list, err := s.tmpl.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (s *Server) handleUpsertTemplate(w http.ResponseWriter, r *http.Request) {
	if s.tmpl == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("templates unavailable"))
		return
	}
	var req templates.UpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	t, err := s.tmpl.Upsert(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "template.upsert", t.ID, map[string]any{"name": t.Name})
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if s.tmpl == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("templates unavailable"))
		return
	}
	id := r.PathValue("id")
	if err := s.tmpl.Delete(id); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	s.audit(r, "template.delete", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStartTemplate(w http.ResponseWriter, r *http.Request) {
	if s.tmpl == nil || s.sess == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("unavailable"))
		return
	}
	t, err := s.tmpl.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// ensure context if requested
	if t.EnsureCtx {
		abs, rel, err := s.resolveMapCwd(t.Cwd)
		if err == nil {
			_ = abs
			// reuse prepare path via soft ensure
			s.prepareSessionContext(&session.CreateRequest{Cwd: rel, Prompt: t.Prompt})
		}
	}
	sess, err := s.sess.Create(session.CreateRequest{
		Agent:       t.Agent,
		Cwd:         t.Cwd,
		Name:        t.Name,
		Prompt:      t.Prompt,
		Account:     t.Account,
		AccountMode: t.AccountMode,
		Mode:        session.ModeTTY,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "template.start", t.ID, map[string]any{"session_id": sess.ID})
	writeJSON(w, http.StatusCreated, sess)
}

// —— Audit ——

func (s *Server) handleAuditTail(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}})
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.auditLog.Tail(n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list, "path": s.auditLog.Path()})
}

// —— Backup ——

func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	path, err := backup.Create(s.cfg.JobsDir, "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.audit(r, "backup.create", path, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("path required"))
		return
	}
	// safety: only under jobs_dir
	if !strings.HasPrefix(filepath.Clean(body.Path), filepath.Clean(s.cfg.JobsDir)) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("path must be under jobs_dir"))
		return
	}
	if err := backup.Restore(s.cfg.JobsDir, body.Path); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.audit(r, "backup.restore", body.Path, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// —— Skills install helper ——

func (s *Server) handleSkillsInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cwd string `json:"cwd"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	abs, rel, err := s.resolveMapCwd(body.Cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Write/refresh .agents/INSTRUCTIONS.md via context ensure files
	writeFiles := true
	autoIndex := false
	if _, err := s.ctxEnsure(abs, rel, writeFiles, autoIndex); err != nil {
		// soft
		s.log.Warn("skills ensure", "err", err)
	}
	// Always write a short skill stub into .agents/skills/project-map.md
	skillDir := filepath.Join(abs, ".agents", "skills")
	_ = os.MkdirAll(skillDir, 0o755)
	stub := "# Project map skill\n\n" +
		"Read `.agents/PROJECT_MAP.md` and `.agents/CONTEXT.md` before exploring.\n" +
		"Regenerate: agentsctl context ensure -r " + rel + "\n" +
		"Search: agentsctl memory search -r " + rel + " \"query\"\n"
	_ = os.WriteFile(filepath.Join(skillDir, "project-map.md"), []byte(stub), 0o644)
	// symlink-friendly: also drop one-liner into AGENTS.md append marker if missing
	agentsMD := filepath.Join(abs, "AGENTS.md")
	marker := "## Project map (agentsd)"
	if b, err := os.ReadFile(agentsMD); err == nil {
		if !strings.Contains(string(b), marker) {
			f, err := os.OpenFile(agentsMD, os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				_, _ = f.WriteString("\n\n" + marker + "\nRead `.agents/PROJECT_MAP.md` before exploring. Regenerate: `agentsctl context ensure`.\n")
				_ = f.Close()
			}
		}
	}
	s.audit(r, "skills.install", rel, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"cwd":  rel,
		"path": filepath.Join(".agents", "skills", "project-map.md"),
	})
}

// —— Workspace tasks (<cwd>/.agents/tasks.json) ——

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	cwd := r.URL.Query().Get("cwd")
	abs, rel, err := s.resolveMapCwd(cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	list, err := workspaces.ListTasks(abs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if list == nil {
		list = []workspaces.Task{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":     rel,
		"cwd_abs": abs,
		"path":    filepath.Join(".agents", workspaces.TasksFile),
		"tasks":   list,
	})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cwd   string `json:"cwd"`
		Title string `json:"title"`
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
	t, err := workspaces.CreateTask(abs, body.Title)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "task.create", t.ID, map[string]any{"cwd": rel, "title": t.Title})
	writeJSON(w, http.StatusCreated, map[string]any{
		"cwd":   rel,
		"task":  t,
		"tasks": mustListTasks(abs),
	})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Cwd    string  `json:"cwd"`
		Status *string `json:"status"`
		Title  *string `json:"title"`
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
	var st *workspaces.TaskStatus
	if body.Status != nil {
		v := workspaces.TaskStatus(strings.TrimSpace(*body.Status))
		st = &v
	}
	t, err := workspaces.UpdateTask(abs, id, st, body.Title)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "task.update", id, map[string]any{"cwd": rel, "status": t.Status})
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":   rel,
		"task":  t,
		"tasks": mustListTasks(abs),
	})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cwd := r.URL.Query().Get("cwd")
	if cwd == "" && r.Method == http.MethodPost {
		var body struct {
			Cwd string `json:"cwd"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cwd = body.Cwd
	}
	abs, rel, err := s.resolveMapCwd(cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := workspaces.DeleteTask(abs, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "task.delete", id, map[string]any{"cwd": rel})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"cwd":   rel,
		"id":    id,
		"tasks": mustListTasks(abs),
	})
}

func mustListTasks(abs string) []workspaces.Task {
	list, err := workspaces.ListTasks(abs)
	if err != nil || list == nil {
		return []workspaces.Task{}
	}
	return list
}

// —— Workspace dashboard ——

func (s *Server) handleWorkspaceDashboard(w http.ResponseWriter, r *http.Request) {
	list := workspaces.List(s.cfg)
	type card struct {
		Path         string `json:"path"`
		Abs          string `json:"abs"`
		MapExists    bool   `json:"map_exists"`
		MapStale     bool   `json:"map_stale"`
		HasContext   bool   `json:"has_context"`
		MemoryDocs   int    `json:"memory_docs"`
		LiveSessions int    `json:"live_sessions"`
	}
	var cards []card
	liveByCwd := map[string]int{}
	if s.sess != nil {
		if sessions, err := s.sess.List(); err == nil {
			for _, sess := range sessions {
				if sess.State == session.StateRunning {
					liveByCwd[sess.Cwd]++
				}
			}
		}
	}
	for _, e := range list {
		c := card{Path: e.Path, Abs: e.Abs, LiveSessions: liveByCwd[e.Path]}
		st := projmap.ReadStatus(e.Abs)
		c.MapExists = st.Exists
		c.MapStale = st.Stale
		if _, err := os.Stat(filepath.Join(e.Abs, ".agents", "CONTEXT.md")); err == nil {
			c.HasContext = true
		}
		if s.mem != nil {
			n, _ := s.mem.Stats(e.Path)
			c.MemoryDocs = n
		}
		cards = append(cards, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspaces": cards,
		"generated":  time.Now().UTC(),
	})
}

func (s *Server) ctxEnsure(abs, rel string, writeFiles, autoIndex bool) (ctxmgr.EnsureResult, error) {
	return ctxmgr.New(s.mem).Ensure(abs, rel, ctxmgr.Options{
		AutoIndex:  &autoIndex,
		WriteFiles: &writeFiles,
		PackBudget: s.cfg.ContextPackBudget(),
	})
}

// history search across session .pane files
func (s *Server) handleHistorySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("q required"))
		return
	}
	type hit struct {
		SessionID string `json:"session_id"`
		Agent     string `json:"agent,omitempty"`
		Cwd       string `json:"cwd,omitempty"`
		Name      string `json:"name,omitempty"`
		Snippet   string `json:"snippet,omitempty"`
		Source    string `json:"source"` // history|recording
	}
	var hits []hit
	ql := strings.ToLower(q)
	if s.sess != nil {
		sessions, _ := s.sess.List()
		for _, sess := range sessions {
			data, source, err := s.sess.History(sess.ID)
			if err != nil || len(data) == 0 {
				continue
			}
			plain := strings.ToLower(string(data))
			if !strings.Contains(plain, ql) {
				continue
			}
			// snippet
			idx := strings.Index(plain, ql)
			start := idx - 40
			if start < 0 {
				start = 0
			}
			end := idx + len(q) + 60
			if end > len(data) {
				end = len(data)
			}
			hits = append(hits, hit{
				SessionID: sess.ID,
				Agent:     sess.Agent,
				Cwd:       sess.Cwd,
				Name:      sess.Name,
				Snippet:   string(data[start:end]),
				Source:    source,
			})
			if len(hits) >= 40 {
				break
			}
		}
	}
	if s.rec != nil && len(hits) < 40 {
		metas, _ := s.rec.Search(q, 40-len(hits))
		for _, m := range metas {
			hits = append(hits, hit{
				SessionID: m.SessionID,
				Agent:     m.Agent,
				Cwd:       m.Cwd,
				Name:      m.Name,
				Snippet:   "recording " + m.ID,
				Source:    "recording",
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "hits": hits})
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	if s.notify == nil || !s.notify.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("notify.webhook_url not configured"))
		return
	}
	s.notify.Emit("notify.test", "", "", "", "", "test event from agentsd", map[string]any{
		"actor": s.actor(r),
	})
	s.audit(r, "notify.test", "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleUploadImage saves a clipboard/drag image into the session workspace.
// Body: { "cwd": "agents", "mime": "image/png", "data": "<base64 or data URL>" }
// Optional session_id uses that session's cwd when cwd is empty.
func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cwd       string `json:"cwd"`
		SessionID string `json:"session_id"`
		MIME      string `json:"mime"`
		Data      string `json:"data"`
		// Filename hint ignored for safety; server generates names
		Name string `json:"name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cwd := strings.TrimSpace(body.Cwd)
	if cwd == "" && body.SessionID != "" && s.sess != nil {
		if sess, err := s.sess.Get(body.SessionID); err == nil {
			cwd = sess.Cwd
		}
	}
	if cwd == "" {
		cwd = s.cfg.DefaultCwd
	}
	if cwd == "" {
		cwd = "."
	}
	abs, rel, err := s.resolveMapCwd(cwd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := uploads.SaveImage(s.cfg.WorkspaceRoot, abs, rel, body.MIME, body.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.audit(r, "upload.image", rel, map[string]any{
		"bytes": res.Bytes,
		"mime":  res.MIME,
		"path":  res.CwdRel,
	})
	// Prefer absolute path for agent CLIs (unambiguous); also return cwd-relative.
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"abs":     res.Abs,
		"rel":     res.Rel,
		"cwd_rel": res.CwdRel,
		"bytes":   res.Bytes,
		"mime":    res.MIME,
		// Path to paste into the terminal (absolute, shell-safe if no spaces issues)
		"paste": res.Abs,
	})
}
