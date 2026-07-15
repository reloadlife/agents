# Web UI

`agentsd` can serve an **embedded browser UI** for managing interactive agent sessions.

## What it does

- Log in with the same `AGENTSD_TOKEN` used by `agentsctl`
- **Sessions-first rail**: filter, open; Stop/Resume/Delete via row overflow menu
- Start sessions via in-shell **New session** page (agent / workspace / optional label + seed prompt; worktree toggle; account under progressive disclosure)
- **Projects** list (`/projects`): workspaces with session counts and quick actions
- **New project** page: clone or fork into `workspace_root`
- **Git Changes** full page (`/changes`, `/git`): status, diffs, commit/PR, worktrees panel
- **Account switcher** (cursor-switch): Settings → Agent accounts
- Multi-tab **xterm.js** over PTY WebSocket (`/v1/sessions/{id}/pty`) with connection pill
- **Tools** page + Settings → Workspace (shared context/map/memory/playwright)
- Session history / recordings via API (recording store is opt-in; see [RECORDING.md](./RECORDING.md))
- **Command palette** (`⌘K` / Ctrl+K) for navigate · workspace · host · appearance
- Keyboard: `j/k` list, `⇧j/⇧k` step+open, `h/l` tabs, `Ctrl+Tab` tabs (works in TTY), `1–9` jump tab, `y` copy id, `n`/`⇧n` new, `?` help

### Background sessions

Closing a browser tab, switching tabs, or disconnecting **does not** kill the agent.
Only **Kill** (or `agentsctl session kill`) stops the tmux session.

This is the same lifecycle as the CLI: detach = leave running; kill = stop.

**Resume** (UI ↻ or click a non-running session, or `agentsctl session resume ID`):

- If tmux is still alive (e.g. after agentsd restart with `KillMode=process`): re-attach
- If the agent is gone (host reboot, kill): restart under the same control-plane id/agent/cwd and pass the agent CLI’s own resume flags when we know the conversation id  
  - **grok**: `grok --session-id <uuid>` on create → `grok --resume <uuid>` on resume  
  - **claude** / **codex** / **opencode**: `--resume` / `resume` / `--session` when id known, else continue/last for that cwd  
  - Terminal scrollback is still restored from the pane snapshot

### Worktrees

New session can create an isolated **git worktree** (default on in the UI when the
workspace is a git repo). Session list shows a `wt` badge. Git page lists linked
worktrees and can start a session from one. API: `POST /v1/sessions` with worktree
fields; `GET /v1/git/worktrees?cwd=`.

## Enable / disable

Default: **enabled**.

```toml
[web]
enabled = true   # set false for API-only
```

Open `http://<listen>/` (e.g. `http://127.0.0.1:8787/`).

### Client routes

The SPA uses real paths (History API). Unknown non-asset paths fall back to `index.html`.

| Path | View |
|------|------|
| `/` or `/new` | New session (in-shell page) |
| `/project/new` · `/projects/new` · `/new/project` | New project (clone/fork) |
| `/projects` · `/project` | **Projects list** (workspaces + session counts) |
| `/desk` | Empty desk (no modal) |
| `/tools` | Global tools page |
| `/git` · `/changes` | Git changes page (diff, commit, worktrees) |
| `/remote` · `/open` | Open remote editor commands |
| `/help` · `/shortcuts` | Keyboard shortcuts |
| `/profile` · `/profile/{tab}` | Profile / settings (`accounts`, `github`, `ssh`, `workspace`, `about`) |
| `/settings` · `/settings/{tab}` | Alias for profile |
| `/project/{projectId}/session/{sessionId}` | Open session (deep-linkable) |
| `/project/{projectId}/session/{sessionId}/tools` | Session + tools for that project |

`projectId` is the workspace cwd (`agents`, nested `foo~bar`, `_` for `.`). Session list rows and tabs are real links (middle-click / copy URL works).

In-shell pages keep the sidebar + topbar; the terminal pane is replaced by the page body (same pattern for Tools, New session, New project, Remote, Help, Git).

### Open from CLI (auto-login)

```bash
agentsctl web                 # open browser + inject token from config
agentsctl web --print         # print login URL only
agentsctl web --no-auth       # open without embedding token
agentsctl -u http://agents:8787 web
```

The CLI builds `http://…/#token=…`. The SPA reads the **hash fragment** (not sent to the server), writes localStorage, then strips the fragment from the address bar.

## Auth

| Path | Auth |
|------|------|
| `/`, SPA routes, `/assets/*` | Public (static shell only) |
| `/healthz` | Public |
| `/v1/*` | Bearer token (header or `?token=` for WebSocket) |

The SPA stores the token in **localStorage** and sends:

- `Authorization: Bearer …` on REST
- `?token=` on the PTY WebSocket

One-shot login also accepts `#token=` / `#access_token=` / `?token=` (stripped after consume).

Multi-token servers accept any configured token the same way. Treat the token like shell access to agent tools on that host (see [SECURITY.md](../SECURITY.md)). Prefer localhost + Tailscale / tunnel, not a raw public bind.

## Recordings

When `sessions.recording = true` on the server, pane archives are available via
`/v1/recordings` (CLI: `agentsctl recordings …`). Privacy defaults and retention:
[RECORDING.md](./RECORDING.md). The web shell may call the same APIs when panels
are enabled; auth is always bearer-gated.

## Develop the UI

Source: `web/` (Vite + TypeScript + xterm.js).

```bash
make web          # bun or npm → internal/webui/dist
make build        # embeds dist into agentsd
```

Dev server with API proxy (agentsd already on `:8787`):

```bash
cd web && bun install && bun run dev
# → http://127.0.0.1:5173  (proxies /v1 and /healthz)
```

Committed `internal/webui/dist` lets pure-Go builds work without Node; rebuild with `make web` when you change `web/`. CI runs `make web` before Go tests/build.

## Related

- [ARCHITECTURE.md](./ARCHITECTURE.md) — PTY + session lifecycle + worktrees
- [RECORDING.md](./RECORDING.md) — session recording privacy
- [REMOTE-TTY.md](./REMOTE-TTY.md) — CLI remote TTY
