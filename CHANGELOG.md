# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
