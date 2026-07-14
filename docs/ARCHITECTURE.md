# Architecture

## Components

```text
┌─────────────────────────────────────────────────────────────┐
│ clients                                                     │
│  agentsctl — REST + WS PTY + Bubble Tea TUI                 │
│  browser   — embedded SPA (xterm.js multi-tab) at GET /     │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP + WebSocket + Bearer token
┌───────────────────────────▼─────────────────────────────────┐
│ agentsd (server)                                            │
│  auth middleware (/v1/*); static web UI public              │
│  session manager → tmux new-session -d -c <cwd> -- <agent>  │
│  PTY handler → pty.Start(tmux attach) ↔ WebSocket           │
│  optional job queue → headless print runners                │
│  go:embed web UI (internal/webui/dist)                      │
│  project maps → <cwd>/.agents/PROJECT_MAP.md                │
│  memory (SQLite FTS5 under jobs_dir/memory)                 │
└─────────────────────────────────────────────────────────────┘
```

Browser and CLI share the same session API. Detaching a PTY (closing a tab or
`agentsctl` disconnect) leaves the tmux agent running until kill.

## Session lifecycle

1. `POST /v1/sessions` with `{ agent, cwd, mode: "tty" }`
2. Server resolves `cwd` under `workspace_root` via allowlist
3. Resolves agent binary (`LookPath` + common install dirs)
4. `tmux new-session -d -s la-<id> -c <abs> -- <bin> <args…>` (setsid; outside agentsd process group)
5. Metadata persisted under `jobs_dir/sessions/<id>.json`
6. Optional seed prompt via `tmux send-keys` (not CLI `-p`)
7. Client attaches with `GET /v1/sessions/{id}/pty` (WebSocket)
8. Server opens a PTY running `tmux attach -t la-<id>`
9. Detach closes the **attach** process; session keeps running until `kill`
10. Browser multi-tab UI attaches the **active** tab only; inactive tabs are UI state, not extra server processes
11. `POST /v1/sessions/{id}/resume` — if tmux still alive, re-mark running; else restart agent with same id/agent/cwd (process restart, not in-agent chat restore)
12. **Terminal history:** each session uses a large tmux `history-limit`; PTY attach seeds the client with `tmux capture-pane -e -S -` (full scrollback). On kill/detach, a snapshot is written to `jobs_dir/sessions/<id>.pane`. After death, `GET /v1/sessions/{id}/history` serves that file; resume+attach prepends it above the new process output.

Note: this is **terminal scrollback**, not Claude/Grok conversation memory. Agent-internal chat state dies with the process unless the agent CLI itself supports continue/resume.

### Surviving `agentsd` restart

Agent processes live in **tmux**, not as children of the PTY attach. Metadata is on disk. After agentsd restarts it reloads JSON and probes `tmux has-session`.

**Caveat:** systemd’s default `KillMode=control-group` kills *everything* in the service cgroup on stop/restart — including the tmux server that was first started from agentsd. Deploy units set `KillMode=process` so only agentsd is signaled. Reload units after upgrade (`systemctl daemon-reload`).

If tmux was wiped anyway (old unit, manual `tmux kill-server`, host reboot), sessions show as `exited` — use **resume** to start a fresh agent under the same session id.

## Why tmux

- Survives client disconnects  
- Survives agentsd restart (with `KillMode=process`)  
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
