# Architecture

## Components

```text
┌─────────────────────────────────────────────────────────────┐
│ clients                                                     │
│  agentsctl — REST + WS PTY + Bubble Tea TUI                 │
│  browser   — embedded SPA (xterm.js multi-tab) at GET /     │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP + WebSocket + Bearer token(s)
┌───────────────────────────▼─────────────────────────────────┐
│ agentsd (server)                                            │
│  auth middleware (/v1/*): multi-token + optional trusted hdr│
│  session manager → tmux new-session -d -c <cwd> -- <agent>  │
│  optional git worktree per session                          │
│  PTY handler → pty.Start(tmux attach) ↔ WebSocket           │
│  optional job queue → headless print runners                │
│  go:embed web UI (internal/webui/dist)                      │
│  project maps → <cwd>/.agents/PROJECT_MAP.md                │
│  memory (SQLite FTS5 under jobs_dir/memory)                 │
│  optional recordings under jobs_dir/recordings              │
└─────────────────────────────────────────────────────────────┘
```

Browser and CLI share the same session API. Detaching a PTY (closing a tab or
`agentsctl` disconnect) leaves the tmux agent running until kill.

## Session lifecycle

1. `POST /v1/sessions` with `{ agent, cwd, mode: "tty", … }`  
   - Optional **worktree**: create/checkout an isolated git worktree under the repo (branch often `agents/<short-id>` or a client-supplied name). Session records `worktree`, `worktree_path`, `base_cwd`, `branch`.  
   - Optional **new workspace directory** via prior `POST /v1/workspaces` `{ "name" }`.
2. Server resolves `cwd` under `workspace_root` via allowlist
3. Resolves agent binary (`LookPath` + common install dirs)
4. `tmux new-session -d -s la-<id> -c <abs> -- <bin> <args…>` (setsid; outside agentsd process group)
5. Metadata persisted under `jobs_dir/sessions/<id>.json`
6. Optional seed prompt via `tmux send-keys` (not CLI `-p`)
7. Client attaches with `GET /v1/sessions/{id}/pty` (WebSocket)
8. Server opens a PTY running `tmux attach -t la-<id>`
9. Detach closes the **attach** process; session keeps running until `kill`
10. Browser multi-tab UI attaches the **active** tab only; inactive tabs are UI state, not extra server processes
11. `POST /v1/sessions/{id}/resume` — if tmux still alive, re-mark running; else restart agent with same control-plane id/agent/cwd **and** native conversation resume when supported:
    - **grok**: create with `--session-id <uuid>` (stored as `agent_session_id`); resume with `--resume <uuid>` (or `--continue` / discover last under `~/.grok/sessions/` for legacy)
    - **claude**: resume with `--resume <id>` when known, else `--continue`
    - **codex**: `codex resume <id>` or `resume --last`
    - **opencode**: `--session <id>` or `--continue`
12. **Terminal history / session preview:** each session uses a large tmux `history-limit`; PTY attach seeds the client with `tmux capture-pane -e -S -` (full scrollback). On kill/detach, a snapshot is written to `jobs_dir/sessions/<id>.pane`. After death, `GET /v1/sessions/{id}/history` serves that file (plain text or `?format=json`); resume+attach prepends it above the new process output.
13. **Opt-in recording:** if `sessions.recording = true`, pane data is also archived under `jobs_dir/recordings/` for multi-id list/search. Default **off**. See [RECORDING.md](./RECORDING.md).

Chat memory is **agent-native** (CLI session store). Terminal scrollback is separate (tmux / `.pane` files / optional recording store).

### Surviving `agentsd` restart

Agent processes live in **tmux**, not as children of the PTY attach. Metadata is on disk. After agentsd restarts it reloads JSON and probes `tmux has-session`.

**Caveat:** systemd’s default `KillMode=control-group` kills *everything* in the service cgroup on stop/restart — including the tmux server that was first started from agentsd. Deploy units set `KillMode=process` so only agentsd is signaled. Reload units after upgrade (`systemctl daemon-reload`).

If tmux was wiped anyway (old unit, manual `tmux kill-server`, host reboot), sessions show as `exited` — use **resume** to start a fresh agent under the same session id.

## Worktrees

When a session requests a worktree (UI default for git repos; API fields on create):

- agentsd creates a linked worktree for an agent branch (or uses an existing path)
- The agent `cwd` becomes the worktree path; `base_cwd` remains the main workspace
- `GET /v1/git/worktrees?cwd=` lists worktrees for the Git UI
- Isolation is **filesystem/git**, not multi-tenant security — same API token still owns all sessions

## Why tmux

- Survives client disconnects  
- Survives agentsd restart (with `KillMode=process`)  
- Multiple attaches possible  
- Mature terminal multiplexing  
- Agent CLIs already assume a real TTY  

## Auth model

| Mechanism | Role |
|-----------|------|
| Primary bearer (`auth.bearer_env` → label `default`) | Required by default on all `/v1/*` |
| `extra_tokens` (label → env var) | Additional accepted bearers; labels appear in audit actor |
| `?token=` query | Same as bearer (WebSocket-friendly) |
| `trusted_header` | Identity for audit; can authenticate alone only if `require_bearer = false` |
| Public paths | `/healthz` + non-`/v1` static web shell |

Details: [SECURITY.md](../SECURITY.md).

## Print jobs (secondary)

`POST /v1/jobs` runs configured `print_args` (e.g. `claude -p`) with cleaned env, concurrency limit, SSE logs. Prefer TTY sessions for subscription tools.

## Trust boundaries

| Boundary | Mechanism |
|----------|-----------|
| Network | bind address + reverse proxy / tunnel |
| API | bearer token map (+ optional trusted header) |
| Filesystem | `workspace_root` + allowlist |
| Process | agent binary only; no free-form shell API in v0 |
| Recordings | same API auth as sessions; default off |
