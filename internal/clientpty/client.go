package clientpty

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

const (
	// maxReconnects limits automatic reconnect attempts after a dropped PTY.
	maxReconnects = 8
	// reconnectBase is the first backoff delay.
	reconnectBase = 500 * time.Millisecond
)

// Attach connects to agentsd PTY websocket and bridges the local terminal (full TTY).
// On transient disconnects it reconnects with a short status banner (session keeps running).
// baseURL like http://192.168.20.6:8787 , sessionID like s_01...
func Attach(baseURL, token, sessionID string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("stdin is not a terminal — run from a real TTY")
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("make raw: %w", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	wsURL, err := ptyURL(baseURL, sessionID)
	if err != nil {
		return err
	}

	attempt := 0
	for {
		err = attachOnce(wsURL, token, attempt > 0)
		if err == nil || err == errCleanExit {
			return nil
		}
		if isFatalAttach(err) {
			return err
		}
		// unexpected disconnect — reconnect
		attempt++
		if attempt > maxReconnects {
			return fmt.Errorf("pty disconnected after %d reconnects: %w", maxReconnects, err)
		}
		delay := reconnectBase * time.Duration(1<<min(attempt-1, 4))
		banner(fmt.Sprintf("\r\n\x1b[33m[agentsctl] connection lost (%v) — reconnecting in %s… (%d/%d)\x1b[0m\r\n",
			briefErr(err), delay.Round(time.Millisecond), attempt, maxReconnects))
		time.Sleep(delay)
	}
}

// errCleanExit is returned when the remote PTY signals a normal detach/exit.
var errCleanExit = fmt.Errorf("pty clean exit")

func isFatalAttach(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// auth / session missing / remote error — don't loop
	if strings.Contains(s, "401") || strings.Contains(s, "403") ||
		strings.Contains(s, "404") || strings.Contains(s, "410") ||
		strings.Contains(s, "session not found") || strings.Contains(s, "session not running") ||
		strings.Contains(s, "tmux session gone") || strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "remote: ") {
		return true
	}
	return false
}

func briefErr(err error) string {
	s := err.Error()
	if len(s) > 80 {
		return s[:77] + "…"
	}
	return s
}

func banner(msg string) {
	_, _ = os.Stderr.WriteString(msg)
}

func ptyURL(baseURL, sessionID string) (*url.URL, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/sessions/" + sessionID + "/pty"
	return u, nil
}

type wsConn struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsConn) write(mt int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return w.conn.WriteMessage(mt, data)
}

func attachOnce(base *url.URL, token string, isReconnect bool) error {
	u := *base
	cols, rows := 120, 40
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			cols, rows = w, h
		}
	}
	q := u.Query()
	q.Set("cols", fmt.Sprintf("%d", cols))
	q.Set("rows", fmt.Sprintf("%d", rows))
	u.RawQuery = q.Encode()

	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 15 * time.Second,
	}
	raw, resp, err := dialer.Dial(u.String(), hdr)
	if err != nil {
		if resp != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			return fmt.Errorf("pty dial: %w (%s %s)", err, resp.Status, strings.TrimSpace(string(b)))
		}
		return fmt.Errorf("pty dial: %w", err)
	}
	wc := &wsConn{conn: raw}
	defer raw.Close()

	if isReconnect {
		banner("\r\n\x1b[32m[agentsctl] reconnected — session still running\x1b[0m\r\n")
	}

	errCh := make(chan error, 4)
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }

	// Client keepalive: answer server pings; also send client pings so idle
	// middleboxes don't drop the socket when the agent TUI is quiet.
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				b, _ := json.Marshal(map[string]string{"type": "ping"})
				if err := wc.write(websocket.TextMessage, b); err != nil {
					errCh <- err
					closeDone()
					return
				}
			}
		}
	}()

	// local stdin -> ws
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := os.Stdin.Read(buf)
			if n > 0 {
				if werr := wc.write(websocket.BinaryMessage, buf[:n]); werr != nil {
					errCh <- werr
					closeDone()
					return
				}
			}
			if rerr != nil {
				// local stdin closed (user closed terminal) — stop cleanly
				if rerr == io.EOF {
					errCh <- errCleanExit
				} else {
					errCh <- rerr
				}
				closeDone()
				return
			}
		}
	}()

	// ws -> local stdout
	go func() {
		for {
			mt, data, rerr := raw.ReadMessage()
			if rerr != nil {
				// network / server drop — reconnectable unless normal close after exit
				errCh <- rerr
				closeDone()
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				if _, werr := os.Stdout.Write(data); werr != nil {
					errCh <- werr
					closeDone()
					return
				}
			case websocket.TextMessage:
				var c struct {
					Type string `json:"type"`
					Msg  string `json:"message"`
					Code int    `json:"code"`
				}
				if json.Unmarshal(data, &c) == nil && c.Type != "" {
					switch c.Type {
					case "error":
						errCh <- fmt.Errorf("remote: %s", c.Msg)
						closeDone()
						return
					case "exit":
						// remote attach process exited (detach) — clean, no reconnect
						errCh <- errCleanExit
						closeDone()
						return
					case "ready":
						// ignore; resize follows
					case "ping":
						b, _ := json.Marshal(map[string]string{"type": "pong"})
						_ = wc.write(websocket.TextMessage, b)
					case "pong":
						// idle keepalive ack
					default:
						// unknown ctrl — ignore
					}
					continue
				}
				// unknown text — write through
				_, _ = os.Stdout.Write(data)
			}
		}
	}()

	// resize on SIGWINCH
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-winch:
				sendResize(wc)
			}
		}
	}()
	// initial resize after connect
	sendResize(wc)

	err = <-errCh
	closeDone()
	if err == nil || err == errCleanExit {
		return errCleanExit
	}
	// normal websocket close without explicit exit ctrl → treat as unexpected (reconnect)
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return fmt.Errorf("connection closed: %w", err)
	}
	return err
}

func sendResize(wc *wsConn) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}
	w, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return
	}
	b, _ := json.Marshal(map[string]any{
		"type": "resize",
		"cols": w,
		"rows": h,
	})
	_ = wc.write(websocket.TextMessage, b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
