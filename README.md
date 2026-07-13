# local-agents

**Remote control plane for AI coding CLIs** — run Claude Code, Grok, Codex, OpenCode, or Cursor Agent on a server and use them from your laptop with a **full terminal (PTY)** over the network.

No SSH required for day-to-day use (SSH remains an optional fallback).

```text
  laptop                          server
 ┌─────────────┐    WebSocket    ┌──────────────────────────┐
 │ agentsctl   │ ──────────────► │ agentsd                  │
 │  · tui      │    + REST API   │  · tmux sessions         │
 │  · session  │                 │  · claude/grok/codex/…   │
 │  · status   │                 │  · full PTY bridge       │
 └─────────────┘                 └──────────────────────────┘
```

| Binary | Role |
|--------|------|
| **`agentsd`** | Daemon: HTTP API, session supervisor, PTY WebSocket |
| **`agentsctl`** | CLI + TUI client |

> **Status:** early public preview (`v0.x`). Useful today for a dedicated “agent box”; not multi-tenant hardened yet. See [SECURITY.md](SECURITY.md).

## Why

Interactive agent tools want a real TTY (subscription login, full UI). Headless `-p` / print modes often burn **API credits** and feel different.

`local-agents` keeps the agent **interactive on the server** and streams the TTY to your machine:

| Mode | Command | Billing / feel |
|------|---------|----------------|
| **TTY (default)** | `session start` / `tui` | Subscription UIs; you drive the agent |
| **Print (opt-in)** | `agentsctl run …` | May use API credits — avoid for normal use |

## Quick start

### 1. Server (machine with the agent CLIs + `tmux`)

```bash
# build
git clone https://github.com/reloadlife/agents.git
cd agents
make build && make install   # → ~/.local/bin (non-root)

# config (user)
mkdir -p ~/.config/agentsd ~/.local/share/agents
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

Requirements on the server: **Go** (to build), **tmux**, and whichever CLIs you want (`claude`, `grok`, `codex`, `opencode`, `cursor-agent`).

### 2. Client (your laptop)

```bash
make build && make install   # → ~/.local/bin

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
| POST | `/v1/sessions/{id}/kill` | Kill tmux session |
| POST | `/v1/jobs` … | Optional print/API job queue |

Auth: `Authorization: Bearer <token>` on all `/v1/*` routes (including WebSocket).

## Configuration

| File | Purpose |
|------|---------|
| Server `config.toml` | listen, workspace, allowlist, agent binaries — see [config.example.toml](config.example.toml) |
| `/etc/agentsd/env` | `AGENTSD_TOKEN=…` |
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
| [docs/INSTALL.md](docs/INSTALL.md) | Install & deploy |
| [docs/REMOTE-TTY.md](docs/REMOTE-TTY.md) | Client remote TTY guide |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow |
| [CHANGELOG.md](CHANGELOG.md) | Changes |

## Project layout

```text
cmd/agentsd/          daemon
cmd/agentsctl/        CLI + TUI
internal/
  api/                HTTP + WebSocket routes
  session/            tmux sessions + PTY bridge
  clientpty/          agentsctl PTY client
  tui/                Bubble Tea session picker
  job/                optional print-job queue
  pathallow/          workspace allowlist
  auth/               bearer middleware
deploy/               systemd + example server config
scripts/install.sh    from-source installer
```

## Roadmap (honest)

- [x] Multi-agent interactive sessions + remote PTY  
- [x] CLI status / agents catalog / TUI  
- [ ] First-class non-root install & packaging  
- [ ] Optional Tailscale / CF Access identity (beyond shared token)  
- [ ] Multi-user isolation  
- [ ] Session recording (opt-in, documented privacy)  

## License

[GNU Affero General Public License v3.0](LICENSE) (**AGPL-3.0**).

If you run a modified version of `agentsd` as a network service, you must offer the corresponding source to users who interact with it over the network (AGPL §13).
