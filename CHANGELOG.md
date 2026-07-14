# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.1] — 2026-07-14

### Added

- **Keyboard workflow shortcuts** for the Web UI:
  - Session list: `j`/`k`, arrows, `Enter`/`o` open, `/` filter
  - Tabs: `[`/`]`, `1`–`9`, `0` last, `x` close, `Shift+x` close all
  - Lifecycle: `s` stop, `e` resume, `Shift+d` delete, `c` prune, `r` refresh
  - Panels: `n` new, `t` tools, `,`/`a` settings, `g` GitHub, `Shift+k` SSH, `?` help
  - **Alt+** chords work while the terminal is focused; Esc blurs TTY to arm bare keys
  - Settings: bare `1`–`5` or `Alt+Shift+1`–`5` switch settings tabs

## [0.4.0] — 2026-07-14

### Changed

- **Web UI overhaul** — shadcn-style dark zinc design system:
  - Inter + JetBrains Mono, zinc HSL tokens, white primary buttons
  - Cleaner cards, dialogs, badges, session rail, and settings layout
  - Terminal palette aligned with zinc dark
  - Same features; visual language only (vanilla TS, not React rewrite)

## [0.3.7] — 2026-07-14

### Added

- **Full Settings page** in the Web UI (`, ` or **Settings**):
  - Agent accounts (save / global switch / add slots per platform)
  - GitHub CLI logins
  - SSH keys
  - Workspace tools (map / memory / Playwright)
  - About + shortcuts

## [0.3.6] — 2026-07-14

### Added

- **Multi-account agent profiles** via [cursor-account-switcher](https://github.com/reloadlife/cursor-account-switcher):
  - Platforms: cursor, claude, codex, **grok**, vscode
  - `GET /v1/agent-accounts`, save/switch/add
  - Session field `account` + `account_mode`: **`isolated`** (parallel-safe private HOME) or `global` (host-wide switch)
  - New-session UI: account profile picker when the agent maps to a platform
  - Requires `cursor-switch` on the agents host PATH

## [0.3.5] — 2026-07-14

### Added

- **GitHub account manager** (`gh` on the agents host):
  - `GET /v1/gh/accounts`, `POST /v1/gh/login|switch|logout|setup-git`
  - `agentsctl gh status|login|switch|logout|setup-git`
  - Tools panel: list accounts, switch, logout, login with PAT
  - **Tokens never returned** (login accepts write-only token)

## [0.3.4] — 2026-07-14

### Added

- **SSH key manager** for the agents host: `GET/POST /v1/ssh-keys`, `GET/DELETE /v1/ssh-keys/{name}`, `agentsctl ssh-keys list|gen|show|delete`, Tools panel UI
  - Lists identities under server `~/.ssh`, generates ed25519/rsa, copies public keys
  - **Private keys are never returned over the API**

## [0.3.3] — 2026-07-14

### Changed

- **Web UI redesign:** warm “ink workshop” palette (brass chrome on dark paper), serif display wordmark, denser session cards, cleaner empty state and modals; agent brand colors retained

## [0.3.2] — 2026-07-14

### Fixed

- Web UI **Tools / New / Help** modals not painting: harden workspace option rendering, raise overlay z-index, open panel after click settles (no instant dismiss)

## [0.3.1] — 2026-07-14

### Added

- **Session delete:** `DELETE|POST /v1/sessions/{id}/delete`, `agentsctl session delete`, Web UI Stop/Delete
- **Clone project into workspace:** `POST /v1/workspaces/clone`, `agentsctl workspaces clone URL [--fork]`, New-session “New project from git”
- Per-agent brand colors in the Web UI (Claude / Grok / Codex / Cursor / …)

### Fixed

- New session / Tools buttons (event delegation, form reads DOM, toasts above modals)
- Workspace list clients handle `{path}` objects + flat `paths[]`

### Changed

- Web UI palette: muted steel (no neon pulse); clearer session lifecycle actions

## [0.3.0] — 2026-07-14

### Added

- **Embedded Web UI** at `GET /` — multi-tab xterm.js sessions over the existing PTY WebSocket
- Web UI **project map** + **memory** panels (generate/show map, index/search)
- `web/` frontend (Vite + TypeScript + xterm.js); `make web` builds into `internal/webui/dist`
- Config `[web] enabled` (default true) to disable the SPA
- Auth: only `/v1/*` requires bearer token; static UI shell is public
- **Project maps:** `agentsctl map generate|show|status`, `POST/GET /v1/maps`, writes `.agents/PROJECT_MAP.md`
- **Workspace memory:** SQLite FTS5 + optional OpenAI-compatible **vector** embeddings
  - `agentsctl memory index|search|stats` (`--mode auto|fts|vector`)
  - `/v1/memory/*`
- Skill: [skills/project-map/SKILL.md](skills/project-map/SKILL.md)
- Docs: [docs/WEB.md](docs/WEB.md), [docs/PROJECT-MAP.md](docs/PROJECT-MAP.md), [docs/MEMORY.md](docs/MEMORY.md)
- **Self-update:** `agentsctl update` and `agentsd update` download the matching GitHub release tarball and replace the binary in place (`--check`, `--force`, `--version TAG`, `--all`)

### Changed

- **Web UI redesign:** sessions-first rail, new-session modal, tools drawer (map/memory/Playwright), connection pill, filter/prune/kill-from-list, keyboard shortcuts, mobile sidebar, ops-console styling
- **Sessions survive agentsd restart:** systemd units use `KillMode=process`; tmux started with setsid; manager reloads JSON + re-probes live tmux
- **`POST /v1/sessions/{id}/resume`**, `agentsctl session resume`, Web UI resume — re-attach if alive, else restart agent under same id
- **Terminal history:** seed PTY attach with `tmux capture-pane` scrollback; snapshot `.pane` on kill/detach; `GET /v1/sessions/{id}/history` + `agentsctl session history`

## [0.2.2] — 2026-07-13

### Changed

- Archive design/plan docs rewritten with generic examples (historical, not host-specific)
- Pathallow unit tests use generic fixture names (`my-app`, `team/*`, …)

### Security

- Removed remaining personal host/repo identifiers from archived docs and tests

## [0.2.1] — 2026-07-13

### Changed

- Project naming aligned to **agents** (`reloadlife/agents`); README/SECURITY/NOTICE/help text
- `scripts/install.sh` installs from GitHub **release tarballs** (source build fallback)
- Example configs use generic paths only; personal host config removed from tree
- SSH snippet uses placeholders (no real LAN IP)
- OPEN-SOURCE.md refreshed for v0.2.x public preview status
- systemd `WorkingDirectory` → `/var/lib/agents`

### Security

- Scrubbed personal host paths/IPs from committed examples

## [0.2.0] — 2026-07-13

### Added

- **Default CLI UX:** bare `agentsctl` (no subcommand) opens the TUI session picker
- **TUI workspace picker:** press `w` to cycle allowlisted cwds from `/v1/workspaces`
- **PTY reconnect:** client auto-reconnects with status banner after transient drops; mutual keepalive pings
- **Non-root install:** `deploy/agentsd.user.service` + docs for systemd user units under `$HOME`
- **CI integration test:** `go test -tags=integration` for tmux + mock agent session create/list/kill
- Release tarballs named `agents_${ver}_${os}_${arch}.tar.gz` (legacy `local-agents_*` still published)

### Changed

- TUI help/status shows agent + cwd; quick-start uses selected workspace
- INSTALL.md prefers non-root layout

## [0.1.2] — 2026-07-13

### Added

- `agentsctl playwright status|start|stop|restart|install` and `/v1/playwright/*` API
- Server-side Playwright/Xvfb/docker compose management

## [0.1.1] — 2026-07-13

### Added

- Headed browser support for agent sessions via Xvfb (`sessions.display`)
- Session env: `DISPLAY`, `PLAYWRIGHT_HEADLESS=0`, `PLAYWRIGHT_BROWSERS_PATH`, optional `PLAYWRIGHT_SERVER`
- `deploy/xvfb.service` and `scripts/setup-playwright.sh`
- Optional Playwright Docker server (`deploy/docker-compose.playwright.yml`)
- Status panel shows display active/down
- Docs: `docs/PLAYWRIGHT.md`

## [0.1.0] — 2026-07-13

### Added

- Interactive agent sessions via `tmux` (Claude, Grok, Codex, OpenCode, Cursor Agent)
- Full remote PTY over WebSocket (`GET /v1/sessions/{id}/pty`) — no SSH required for TTY
- `agentsctl` CLI with status panel, session commands, Bubble Tea TUI (`agentsctl tui`)
- `agentsctl doctor` — client health checks against agentsd
- `agentsctl workspaces` / `GET /v1/workspaces` — allowlisted cwd browser
- `agentsctl session prune` / `POST /v1/sessions/prune` — clean stopped sessions
- Headless print/API job queue (opt-in; can use API credits)
- Workspace path allowlist, bearer auth, log redaction helpers
- Agent catalog API (`GET /v1/agents`) and status probes (docker, optional GKE)
- Host memory in status (when `/proc/meminfo` available)
- PTY WebSocket write serialization + keepalive pings
- Example systemd unit and deploy configs
- AGPL-3.0 license

### Security

- Bearer token required on all `/v1/*` routes including PTY WebSocket
- Elevated job caps require explicit confirm
