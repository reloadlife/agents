# Architecture

## Components

```text
┌─────────────────────────────────────────────────────────────┐
│ agentsctl (client)                                          │
│  REST: status, agents, sessions CRUD                        │
│  WS:   /v1/sessions/{id}/pty  → raw local terminal          │
│  TUI:  Bubble Tea picker → then PTY attach                  │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP + WebSocket + Bearer token
┌───────────────────────────▼─────────────────────────────────┐
│ agentsd (server)                                            │
│  auth middleware                                            │
│  session manager → tmux new-session -d -c <cwd> -- <agent>  │
│  PTY handler → pty.Start(tmux attach) ↔ WebSocket           │
│  optional job queue → headless print runners                │
└─────────────────────────────────────────────────────────────┘
```

## Session lifecycle

1. `POST /v1/sessions` with `{ agent, cwd, mode: "tty" }`
2. Server resolves `cwd` under `workspace_root` via allowlist
3. Resolves agent binary (`LookPath` + common install dirs)
4. `tmux new-session -d -s la-<id> -c <abs> -- <bin> <args…>`
5. Optional seed prompt via `tmux send-keys` (not CLI `-p`)
6. Client attaches with `GET /v1/sessions/{id}/pty` (WebSocket)
7. Server opens a PTY running `tmux attach -t la-<id>`
8. Binary frames: terminal I/O; JSON text: `{type:resize,cols,rows}`
9. Detach closes the **attach** process; session keeps running until `kill`

## Why tmux

- Survives client disconnects  
- Multiple attaches possible  
- Mature terminal multiplexing  
- Agent CLIs already assume a real TTY  

## Print jobs (secondary)

`POST /v1/jobs` runs configured `print_args` (e.g. `claude -p`) with cleaned env, concurrency limit, SSE logs. Prefer TTY sessions for subscription tools.

## Trust boundaries

| Boundary | Mechanism |
|----------|-----------|
| Network | bind address + reverse proxy / tunnel |
| API | shared bearer token |
| Filesystem | `workspace_root` + allowlist |
| Process | agent binary only; no free-form shell API in v0 |
