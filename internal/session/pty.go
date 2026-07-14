package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Token-authenticated CLI / LAN clients
		return true
	},
}

// ctrl messages (JSON text frames)
type ctrlMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
	Msg  string `json:"message,omitempty"`
}

// wsWriter serializes all WebSocket writes (gorilla requires this).
type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsWriter) write(mt int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
	return w.conn.WriteMessage(mt, data)
}

func (w *wsWriter) writeCtrl(m ctrlMsg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return w.write(websocket.TextMessage, b)
}

// HandlePTY upgrades to WebSocket and bridges a full PTY to `tmux attach`.
func (m *Manager) HandlePTY(w http.ResponseWriter, r *http.Request, id string) {
	s, err := m.Get(id)
	if err != nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	if s.State != StateRunning {
		http.Error(w, fmt.Sprintf(`{"error":"session not running (state=%s)"}`, s.State), http.StatusConflict)
		return
	}
	if err := exec.Command("tmux", "has-session", "-t", s.Tmux).Run(); err != nil {
		http.Error(w, `{"error":"tmux session gone"}`, http.StatusGone)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.log.Error("ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	ww := &wsWriter{conn: conn}

	// Initial size from query or default
	cols, rows := uint16(120), uint16(40)
	if c := r.URL.Query().Get("cols"); c != "" {
		fmt.Sscanf(c, "%d", &cols)
	}
	if rw := r.URL.Query().Get("rows"); rw != "" {
		fmt.Sscanf(rw, "%d", &rows)
	}
	if cols < 20 {
		cols = 20
	}
	if rows < 5 {
		rows = 5
	}

	cmd := exec.Command("tmux", "-u", "attach-session", "-t", s.Tmux)
	cmd.Env = append(os.Environ(),
		"TERM="+envOr("TERM", "xterm-256color"),
		"COLORTERM="+envOr("COLORTERM", "truecolor"),
		"LANG="+envOr("LANG", "en_US.UTF-8"),
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		_ = ww.writeCtrl(ctrlMsg{Type: "error", Msg: err.Error()})
		return
	}
	defer func() {
		_ = ptmx.Close()
		// don't kill tmux session on detach — only the attach process exits
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	// Seed client with scrollback *before* attach stream. Live capture is
	// preferred; if empty, fall back to last on-disk snapshot (post-kill/resume).
	scrollback, src, _ := m.scrollbackForAttach(id, s.Tmux)
	if len(scrollback) > 0 {
		// Chunk large histories so proxies don't choke on huge frames.
		const chunk = 24 * 1024
		for i := 0; i < len(scrollback); i += chunk {
			j := i + chunk
			if j > len(scrollback) {
				j = len(scrollback)
			}
			if err := ww.write(websocket.BinaryMessage, scrollback[i:j]); err != nil {
				return
			}
		}
		m.log.Info("pty scrollback seeded", "id", id, "bytes", len(scrollback), "source", src)
	}

	_ = ww.writeCtrl(ctrlMsg{Type: "ready", Cols: cols, Rows: rows})
	m.log.Info("pty attached", "id", id, "tmux", s.Tmux, "cols", cols, "rows", rows)

	var wg sync.WaitGroup
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }

	// keepalive pings so idle TUIs don't get dropped by proxies
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if err := ww.writeCtrl(ctrlMsg{Type: "ping"}); err != nil {
					closeDone()
					return
				}
			}
		}
	}()

	// PTY -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer closeDone()
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := ww.write(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket -> PTY
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer closeDone()
		_ = conn.SetReadDeadline(time.Time{}) // no idle read timeout; keepalive is server→client
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch mt {
			case websocket.BinaryMessage, websocket.TextMessage:
				if mt == websocket.TextMessage || (len(data) > 0 && data[0] == '{') {
					var c ctrlMsg
					if json.Unmarshal(data, &c) == nil && c.Type != "" {
						switch c.Type {
						case "resize":
							if c.Cols > 0 && c.Rows > 0 {
								_ = pty.Setsize(ptmx, &pty.Winsize{Cols: c.Cols, Rows: c.Rows})
							}
							continue
						case "ping":
							_ = ww.writeCtrl(ctrlMsg{Type: "pong"})
							continue
						case "pong":
							continue
						}
					}
				}
				if _, err := ptmx.Write(data); err != nil {
					return
				}
			case websocket.CloseMessage:
				return
			}
		}
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-done:
	case err := <-waitCh:
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		_ = ww.writeCtrl(ctrlMsg{Type: "exit", Code: code})
	}

	closeDone()
	_ = conn.Close()
	wg.Wait()
	// Snapshot after detach so a later kill/crash still has recent output.
	m.SnapshotHistory(id)
	m.log.Info("pty detached", "id", id)
}

// scrollbackForAttach returns bytes to paint into the client before live attach.
// Prefer live tmux history; on a freshly resumed session fall back to the
// previous snapshot and a visual separator.
func (m *Manager) scrollbackForAttach(id, tmuxName string) (data []byte, source string, err error) {
	live, liveErr := capturePane(tmuxName)
	snap, snapErr := os.ReadFile(m.historyPath(id))

	if liveErr == nil && len(live) > 0 {
		// Keep disk copy fresh for future death.
		_ = os.WriteFile(m.historyPath(id), live, 0o600)
		// Fresh resume: new process is short, old snapshot is long → prepend old output.
		if snapErr == nil && len(snap) > len(live)+64 && !bytesContainsTail(live, snap) {
			sep := []byte("\r\n\r\n\x1b[90m── previous session output (before resume) ──\x1b[0m\r\n\r\n")
			out := make([]byte, 0, len(snap)+len(sep)+len(live)+2)
			out = append(out, snap...)
			if len(snap) > 0 && snap[len(snap)-1] != '\n' {
				out = append(out, '\r', '\n')
			}
			out = append(out, sep...)
			out = append(out, live...)
			return out, "snapshot+live", nil
		}
		return live, "live", nil
	}

	if snapErr == nil && len(snap) > 0 {
		sep := []byte("\r\n\x1b[90m── end of saved history ──\x1b[0m\r\n")
		return append(snap, sep...), "snapshot", nil
	}
	return nil, "", liveErr
}

func bytesContainsTail(hay, needle []byte) bool {
	// cheap: if live already includes end of snap, don't double-prepend
	if len(needle) == 0 || len(hay) == 0 {
		return false
	}
	n := 256
	if len(needle) < n {
		n = len(needle)
	}
	tail := needle[len(needle)-n:]
	return bytes.Contains(hay, tail)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
