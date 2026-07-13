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
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// Attach connects to agentsd PTY websocket and bridges the local terminal (full TTY).
// baseURL like http://192.168.20.6:8787 , sessionID like s_01...
func Attach(baseURL, token, sessionID string) error {
	baseURL = strings.TrimRight(baseURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/sessions/" + sessionID + "/pty"

	// local size
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
	conn, resp, err := dialer.Dial(u.String(), hdr)
	if err != nil {
		if resp != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			return fmt.Errorf("pty dial: %w (%s %s)", err, resp.Status, strings.TrimSpace(string(b)))
		}
		return fmt.Errorf("pty dial: %w", err)
	}
	defer conn.Close()

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("stdin is not a terminal — run from a real TTY")
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("make raw: %w", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// clear screen lightly optional — leave to remote app
	errCh := make(chan error, 2)

	// local stdin -> ws
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := os.Stdin.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errCh <- werr
					return
				}
			}
			if rerr != nil {
				if rerr != io.EOF {
					errCh <- rerr
				} else {
					errCh <- nil
				}
				return
			}
		}
	}()

	// ws -> local stdout
	go func() {
		for {
			mt, data, rerr := conn.ReadMessage()
			if rerr != nil {
				errCh <- rerr
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				if _, werr := os.Stdout.Write(data); werr != nil {
					errCh <- werr
					return
				}
			case websocket.TextMessage:
				var c struct {
					Type string `json:"type"`
					Msg  string `json:"message"`
					Code int    `json:"code"`
				}
				if json.Unmarshal(data, &c) == nil {
					switch c.Type {
					case "error":
						errCh <- fmt.Errorf("remote: %s", c.Msg)
						return
					case "exit":
						errCh <- nil
						return
					case "ready", "pong":
						// ignore
					}
				} else {
					// unknown text — write through
					_, _ = os.Stdout.Write(data)
				}
			}
		}
	}()

	// resize on SIGWINCH
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			sendResize(conn)
		}
	}()
	// initial resize after connect
	sendResize(conn)

	err = <-errCh
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
		!strings.Contains(err.Error(), "use of closed network connection") &&
		!strings.Contains(err.Error(), "EOF") {
		return err
	}
	return nil
}

func sendResize(conn *websocket.Conn) {
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
	_ = conn.WriteMessage(websocket.TextMessage, b)
}
