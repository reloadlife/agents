package job

import "time"

type State string

const (
	StateQueued           State = "queued"
	StateAwaitingConfirm  State = "awaiting_confirm"
	StateRunning          State = "running"
	StateSucceeded        State = "succeeded"
	StateFailed           State = "failed"
	StateCancelled        State = "cancelled"
	StateInterrupted      State = "interrupted"
)

func (s State) Terminal() bool {
	switch s {
	case StateSucceeded, StateFailed, StateCancelled, StateInterrupted:
		return true
	default:
		return false
	}
}

type Job struct {
	ID        string            `json:"id"`
	Title     string            `json:"title,omitempty"`
	Prompt    string            `json:"prompt"`
	Agent     string            `json:"agent"`
	Cwd       string            `json:"cwd"`         // request path (relative)
	CwdAbs    string            `json:"cwd_abs"`     // resolved absolute
	Caps      []string          `json:"caps,omitempty"`
	State     State             `json:"state"`
	Timeout   string            `json:"timeout,omitempty"`
	ExitCode  *int              `json:"exit_code,omitempty"`
	Error     string            `json:"error,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Confirm   string            `json:"-"` // one-time confirm token (not listed publicly by default)
}

type CreateRequest struct {
	Prompt  string   `json:"prompt"`
	Agent   string   `json:"agent"`
	Cwd     string   `json:"cwd"`
	Timeout string   `json:"timeout,omitempty"`
	Caps    []string `json:"caps,omitempty"`
	Title   string   `json:"title,omitempty"`
}

type Result struct {
	ExitCode int    `json:"exit_code"`
	Summary  string `json:"summary,omitempty"`
	Error    string `json:"error,omitempty"`
}
