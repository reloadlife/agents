# agents

**Remote control plane for AI coding CLIs** — run Claude Code, Grok, Codex, OpenCode, or Cursor Agent on a server and use them from your laptop with a **full terminal (PTY)** over the network.

No SSH required for day-to-day use (SSH remains an optional fallback).

```text
  laptop / browser                 server
 ┌─────────────┐    WebSocket    ┌──────────────────────────┐
 │ agentsctl   │ ──────────────► │ agentsd                  │
 │  · tui      │    + REST API   │  · tmux sessions         │
 │  · session  │                 │  · claude/grok/codex/…   │
 │ browser UI  │  multi-tab PTY  │  · full PTY bridge       │
 │  · tabs     │ ──────────────► │  · embedded web UI (/ )  │
 └─────────────┘                 └──────────────────────────┘
```

| Binary | Role |
|--------|------|
| **`agentsd`** | Daemon: HTTP API, session supervisor, PTY WebSocket |
| **`agentsctl`** | CLI + TUI client |

> **Status:** early public preview (`v0.2.x`). Useful today for a dedicated “agent box”; not multi-tenant hardened yet. See [SECURITY.md](SECURITY.md).

## Why

Interactive agent tools want a real TTY (subscription login, full UI). Headless `-p` / print modes often burn **API credits** and feel different.

**agents** keeps the agent **interactive on the server** and streams the TTY to your machine:

| Mode | Command | Billing / feel |
|------|---------|----------------|
| **TTY (default)** | `session start` / `agentsctl` | Subscription UIs; you drive the agent |
| **Print (opt-in)** | `agentsctl run …` | May use API credits — avoid for normal use |

## Install

### Prebuilt binaries (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/reloadlife/agents/main/scripts/install.sh | bash
# → ~/.local/bin/agentsd  ~/.local/bin/agentsctl
# pin: VERSION=v0.2.0 bash …
# force source build: SOURCE=1 bash …
```

Or grab `agents_${ver}_${os}_${arch}.tar.gz` from [Releases](https://github.com/reloadlife/agents/releases).

### Update in place

```bash
agentsctl update              # latest agentsctl
agentsd update                # latest agentsd (restart the service after)
agentsctl update --check      # dry-run
agentsctl update --version v0.2.2
```

### From source

```bash
git clone https://github.com/reloadlife/agents.git
cd agents
make build && make install   # → ~/.local/bin
```

## Quick start

### 1. Server (machine with the agent CLIs + `tmux`)

```bash
# config (user / non-root)
mkdir -p ~/.config/agentsd ~/.local/share/agents
# copy config.example.toml from the repo or a release tarball
cp config.example.toml ~/.config/agentsd/config.toml
# edit workspace_root, allow paths, agents

echo "AGENTSD_TOKEN=$(openssl rand -hex 32)" > ~/.config/agentsd/env
chmod 600 ~/.config/agentsd/env

# run (foreground)
set -a; source ~/.config/agentsd/env; set +a
agentsd serve --config ~/.config/agentsd/config.toml

# or systemd --user (see docs/INSTALL.md)
cp deploy/agentsd.user.service ~/.config/systemd/user/agentsd.service
systemctl --user daemon-reload && systemctl --user enable --now agentsd
```

Requirements on the server: **tmux**, and whichever CLIs you want (`claude`, `grok`, `codex`, `opencode`, `cursor-agent`). Go is only needed to build from source.

### 2. Client (your laptop)

```bash
# install agentsctl (same install.sh or make install)

agentsctl config init
# edit ~/.config/agentsctl/config.toml:
#   url   = "http://YOUR_SERVER:8787"
#   token = "<same AGENTSD_TOKEN>"

agentsctl status
agentsctl agents
agentsctl                        # default: TUI session picker
# or
agentsctl session start -a claude --open
agentsctl session start -a grok --open
```

### 3. Browser UI (optional)

With `agentsd` running, open the embedded UI (same host/port as the API):

```text
http://127.0.0.1:8787/
```

Paste your `AGENTSD_TOKEN`, then start or open sessions in tabs. Closing a tab
**does not** kill the agent (tmux keeps running). See [docs/WEB.md](docs/WEB.md).

```toml
[web]
enabled = true   # default; set false for API-only
```

## CLI cheat sheet

```bash
agentsctl                        # TUI (default) · a agent · w workspace · 1-9 start · enter open
agentsctl status                 # host panel (use --json for raw)
agentsctl agents                 # which CLIs are available on the server
agentsctl workspaces             # allowlisted cwds
agentsctl tui                    # same as bare agentsctl

agentsctl session start -a claude --open
agentsctl session start -a cursor -r my-repo --open
agentsctl session list
agentsctl session open [id]      # full PTY (WebSocket; auto-reconnect)
agentsctl session open id --ssh  # fallback: ssh -t … tmux attach
agentsctl session kill id
agentsctl session delete id   # stop + remove from list
agentsctl session resume id   # re-attach if alive, else restart agent (same id)
agentsctl session history id  # dump terminal scrollback / last snapshot
agentsctl ssh-keys list|gen NAME|show NAME|delete NAME
agentsctl gh status|login|switch|logout|setup-git
```

## HTTP API (v1)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Liveness (no auth) |
| GET | `/v1/status` | Host / docker / optional GKE / agents |
| GET | `/v1/agents` | Configured CLIs + availability |
| POST | `/v1/sessions` | Start interactive session |
| GET | `/v1/sessions` | List |
| GET | `/v1/sessions/{id}` | Detail (`pty_path`, attach hints) |
| GET | `/v1/sessions/{id}/pty` | **WebSocket full PTY** |
| POST | `/v1/sessions/{id}/kill` | Stop agent (session stays in list) |
| POST | `/v1/sessions/{id}/delete` | Stop agent and remove session |
| DELETE | `/v1/sessions/{id}` | Same as delete |
| POST | `/v1/sessions/{id}/resume` | Re-attach if tmux alive; else restart agent |
| POST | `/v1/workspaces/clone` | `git clone` / `gh fork` into workspace |
| GET/POST | `/v1/ssh-keys` | List / generate SSH identities (public only) |
| GET/DELETE | `/v1/ssh-keys/{name}` | Show public key / delete pair |
| GET | `/v1/gh/accounts` | List GitHub CLI accounts on server (no tokens) |
| POST | `/v1/gh/login` | `gh auth login --with-token` (token write-only) |
| POST | `/v1/gh/switch` | Switch active gh account |
| POST | `/v1/gh/logout` | Logout local gh account |
| GET | `/v1/agent-accounts` | Multi-account profiles (`?platform=grok`) |
| POST | `/v1/agent-accounts/save\|switch\|add` | Save / global-switch / register profiles |
| GET | `/v1/sessions/{id}/history` | Terminal scrollback (live or last snapshot) |
| POST | `/v1/jobs` … | Optional print/API job queue |

Auth: `Authorization: Bearer <token>` on all `/v1/*` routes (including WebSocket).

## Configuration

| File | Purpose |
|------|---------|
| Server `config.toml` | listen, workspace, allowlist, agent binaries — see [config.example.toml](config.example.toml) |
| `~/.config/agentsd/env` or `/etc/agentsd/env` | `AGENTSD_TOKEN=…` |
| `~/.config/agentsctl/config.toml` | client `url`, `token` — `agentsctl config init` |

Important server keys:

```toml
listen = "127.0.0.1:8787"       # prefer localhost + tunnel; use 0.0.0.0 only on trusted LAN
workspace_root = "/home/you/work"
default_cwd = "."
[allow]
paths = [".", "my-repo", "my-repo/**"]
[agents.claude]
bin = "claude"
```

## Headed browsers (Playwright)

Agent sessions can run **non-headless** Chromium via **Xvfb** (`DISPLAY=:99`).

```bash
sudo bash scripts/setup-playwright.sh
sudo cp deploy/xvfb.service /etc/systemd/system/ && sudo systemctl enable --now xvfb
# config: [sessions] display = ":99"
# optional: docker compose -f deploy/docker-compose.playwright.yml up -d
```

See [docs/PLAYWRIGHT.md](docs/PLAYWRIGHT.md).

## Security (read this)

- The token is effectively **remote interactive access** to agent tools on that host.
- Prefer **localhost + Tailscale / Cloudflare Tunnel + Access**, not raw public ports.
- Keep the workspace **allowlist** tight.
- Details: [SECURITY.md](SECURITY.md)

## Docs

| Doc | Contents |
|-----|----------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | How sessions + PTY work |
| [docs/WEB.md](docs/WEB.md) | Embedded multi-tab browser UI |
| [docs/PROJECT-MAP.md](docs/PROJECT-MAP.md) | Project maps for agent orientation |
| [docs/CONTEXT.md](docs/CONTEXT.md) | Context manager (ensure / pack / session seed) |
| [docs/MEMORY.md](docs/MEMORY.md) | Workspace FTS memory for agents |
| [docs/INSTALL.md](docs/INSTALL.md) | Install & deploy |
| [docs/REMOTE-TTY.md](docs/REMOTE-TTY.md) | Client remote TTY guide |
| [docs/OPEN-SOURCE.md](docs/OPEN-SOURCE.md) | Public preview status |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow |
| [CHANGELOG.md](CHANGELOG.md) | Changes |

## Project layout

```text
cmd/agentsd/          daemon
cmd/agentsctl/        CLI + TUI
web/                  browser UI source (Vite + xterm.js)
internal/
  api/                HTTP + WebSocket routes
  webui/              go:embed SPA (built from web/)
  session/            tmux sessions + PTY bridge
  clientpty/          agentsctl PTY client
  tui/                Bubble Tea session picker
  job/                optional print-job queue
  pathallow/          workspace allowlist
  auth/               bearer middleware
deploy/               systemd (user + system) + example configs
scripts/install.sh    release-tarball installer (source fallback)
```

## Roadmap (honest)

- [x] Multi-agent interactive sessions + remote PTY  
- [x] CLI status / agents catalog / TUI defaults  
- [x] Non-root install docs + user systemd unit  
- [x] CI integration test (tmux + mock)  
- [x] Embedded multi-tab Web UI (xterm.js)  
- [x] Project maps (`.agents/PROJECT_MAP.md` + skill)  
- [x] Embedded workspace memory (SQLite FTS5 + optional HTTP embeddings)  
- [x] Context manager (ensure/pack/seed on session + `agentsctl context`)  
- [x] Web UI map + memory panels  
- [ ] Homebrew packaging  
- [ ] Optional Tailscale / CF Access identity (beyond shared token)  
- [ ] Multi-user isolation  
- [ ] Session recording (opt-in, documented privacy)  

## License

[GNU Affero General Public License v3.0](LICENSE) (**AGPL-3.0**).

If you run a modified version of `agentsd` as a network service, you must offer the corresponding source to users who interact with it over the network (AGPL §13).
