# Web UI

`agentsd` can serve an **embedded browser UI** for managing interactive agent sessions.

## What it does

- Log in with the same `AGENTSD_TOKEN` used by `agentsctl`
- **Sessions-first rail**: filter, open; Stop/Resume/Delete via row overflow menu
- Start sessions via Vaul sheet (agent / workspace / optional label + seed prompt; account under progressive disclosure)
- **New project** sheet: clone or fork into `workspace_root`
- **Account switcher** (cursor-switch): Settings → Agent accounts
- Multi-tab **xterm.js** over PTY WebSocket (`/v1/sessions/{id}/pty`) with connection pill
- **Tools** sheet + Settings → Workspace (shared context/map/memory/playwright)
- **Command palette** (`⌘K` / Ctrl+K) for navigate · workspace · host · appearance
- Keyboard: `j/k` list, `⇧j/⇧k` step+open, `h/l` tabs, `Ctrl+Tab` tabs (works in TTY), `1–9` jump tab, `y` copy id, `n`/`⇧n` new, `?` help

### Background sessions

Closing a browser tab, switching tabs, or disconnecting **does not** kill the agent.
Only **Kill** (or `agentsctl session kill`) stops the tmux session.

This is the same lifecycle as the CLI: detach = leave running; kill = stop.

**Resume** (UI ↻ or click a non-running session, or `agentsctl session resume ID`):

- If tmux is still alive (e.g. after agentsd restart with `KillMode=process`): re-attach
- If the agent is gone: start a **new** process with the same session id / agent / cwd  
  (does **not** restore in-agent chat history)

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
| `/` or `/new` | New session modal |
| `/project/new` · `/projects/new` · `/new/project` | New project (clone/fork) modal |
| `/desk` | Empty desk (no modal) |
| `/tools` | Global tools panel |
| `/help` | Keyboard shortcuts |
| `/profile` · `/profile/{tab}` | Profile / settings (`accounts`, `github`, `ssh`, `workspace`, `about`) |
| `/settings` · `/settings/{tab}` | Alias for profile |
| `/project/{projectId}/session/{sessionId}` | Open session (deep-linkable) |
| `/project/{projectId}/session/{sessionId}/tools` | Session + tools for that project |

`projectId` is the workspace cwd (`agents`, nested `foo~bar`, `_` for `.`). Session list rows and tabs are real links (middle-click / copy URL works).

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
| `/`, `/assets/*` | Public (static shell only) |
| `/healthz` | Public |
| `/v1/*` | Bearer token (header or `?token=` for WebSocket) |

The SPA stores the token in **localStorage** and sends:

- `Authorization: Bearer …` on REST
- `?token=` on the PTY WebSocket

One-shot login also accepts `#token=` / `#access_token=` / `?token=` (stripped after consume).

Treat the token like shell access to agent tools on that host (see [SECURITY.md](../SECURITY.md)). Prefer localhost + Tailscale / tunnel, not a raw public bind.

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

Committed `internal/webui/dist` lets pure-Go builds work without Node; rebuild with `make web` when you change `web/`.

## Related

- [ARCHITECTURE.md](./ARCHITECTURE.md) — PTY + session lifecycle
- [REMOTE-TTY.md](./REMOTE-TTY.md) — CLI remote TTY
