# Web UI

`agentsd` can serve an **embedded browser UI** for managing interactive agent sessions.

## What it does

- Log in with the same `AGENTSD_TOKEN` used by `agentsctl`
- **Sessions-first rail**: filter, open, kill, **resume**, prune, copy id
- Start sessions via modal (agent / workspace / optional name + seed prompt)
- **Account profiles** (cursor-switch): pick personal/work/… per agent; isolated = parallel-safe
- **Clone / fork git** into the workspace from the new-session modal (`owner/repo` or full URL)
- Multi-tab **xterm.js** over PTY WebSocket (`/v1/sessions/{id}/pty`) with live connection pill
- **Tools** panel: project map, memory, Playwright, **SSH keys**, **GitHub accounts** (`gh` login/switch/logout; tokens never returned)
- Keyboard shortcuts (`n` new, `t` tools, `/` filter, `1`–`9` tabs, `?` help, `Esc` close)

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

## Auth

| Path | Auth |
|------|------|
| `/`, `/assets/*` | Public (static shell only) |
| `/healthz` | Public |
| `/v1/*` | Bearer token (header or `?token=` for WebSocket) |

The SPA stores the token in **localStorage** and sends:

- `Authorization: Bearer …` on REST
- `?token=` on the PTY WebSocket

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
