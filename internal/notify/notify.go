// Package notify sends optional webhooks for host events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event is a structured notification payload.
type Event struct {
	Type      string         `json:"type"`
	At        time.Time      `json:"at"`
	SessionID string         `json:"session_id,omitempty"`
	Agent     string         `json:"agent,omitempty"`
	Cwd       string         `json:"cwd,omitempty"`
	Name      string         `json:"name,omitempty"`
	Message   string         `json:"message,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// Config for the notifier.
type Config struct {
	// WebhookURL receives POST JSON Event. Empty = disabled.
	WebhookURL string
	// Events filter; empty = all. e.g. session.started, session.stopped, session.killed
	Events []string
}

// Notifier posts events asynchronously.
type Notifier struct {
	cfg Config
	log *slog.Logger
	hc  *http.Client
	mu  sync.Mutex
}

func New(cfg Config, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{
		cfg: cfg,
		log: log,
		hc:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (n *Notifier) Enabled() bool {
	return n != nil && strings.TrimSpace(n.cfg.WebhookURL) != ""
}

func (n *Notifier) allows(typ string) bool {
	if len(n.cfg.Events) == 0 {
		return true
	}
	for _, e := range n.cfg.Events {
		if e == typ || e == "*" {
			return true
		}
	}
	return false
}

// Emit sends the event in the background (never blocks callers long).
func (n *Notifier) Emit(typ string, sessionID, agent, cwd, name, message string, extra map[string]any) {
	if !n.Enabled() || !n.allows(typ) {
		return
	}
	ev := Event{
		Type:      typ,
		At:        time.Now().UTC(),
		SessionID: sessionID,
		Agent:     agent,
		Cwd:       cwd,
		Name:      name,
		Message:   message,
		Extra:     extra,
	}
	go n.post(ev)
}

func (n *Notifier) post(ev Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agentsd-notify/1")
	res, err := n.hc.Do(req)
	if err != nil {
		n.log.Warn("notify webhook failed", "type", ev.Type, "err", err)
		return
	}
	_ = res.Body.Close()
	if res.StatusCode >= 300 {
		n.log.Warn("notify webhook status", "type", ev.Type, "status", res.StatusCode)
	}
}
