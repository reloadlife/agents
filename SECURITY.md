# Security Policy

## What this project is

**agents** (`reloadlife/agents`) is a **remote control plane** for AI coding CLIs (Claude Code, Grok, Codex, OpenCode, Cursor Agent). It can:

- Start interactive agent sessions in `tmux`
- Bridge a **full remote PTY** over WebSocket to your laptop
- Optionally run headless “print” jobs

That is powerful. Treat the API token like a root shell on the machine that runs `agentsd`.

## Supported versions

Latest tagged release on `main` (currently `v0.2.x`). Security fixes land on `main` and ship in the next patch/minor tag.

## Threat model (short)

| Asset | Risk if stolen |
|-------|----------------|
| `AGENTSD_TOKEN` | Create/kill sessions, attach PTY, run print jobs |
| PTY WebSocket | Interactive control of the agent TUI (and tools it can run) |
| Workspace allowlist escape | Read/write outside intended repos |

## Hardening checklist

1. **Bind carefully**
   - Prefer `listen = "127.0.0.1:8787"` + Tailscale / SSH tunnel / Cloudflare Tunnel + Access
   - Avoid `0.0.0.0` on untrusted networks
2. **Token**
   - Generate with `openssl rand -hex 32`
   - Store in `~/.config/agentsd/env` or `/etc/agentsd/env` mode `600` (or your secrets manager)
   - Never commit tokens; rotate if leaked
3. **Workspace allowlist**
   - Keep `[allow].paths` tight
   - Do not set `paths = ["."]` on multi-user hosts unless intentional
4. **Auth on every path**
   - All `/v1/*` routes require bearer token (including WebSocket PTY)
   - Only `/healthz` is public by design
5. **Print jobs (`claude -p`, etc.)**
   - Opt-in only; may burn API credits and run non-interactive agent code
6. **Run as non-root when possible**
   - Prefer `deploy/agentsd.user.service` under your own account

## Reporting a vulnerability

Please open a **private** security advisory on GitHub or email the maintainer via the GitHub profile for [reloadlife](https://github.com/reloadlife). Do not file public issues with exploit details until a fix is available.
