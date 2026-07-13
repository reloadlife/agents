# Security Policy

## What this project is

`local-agents` is a **remote control plane** for AI coding CLIs (Claude Code, Grok, Codex, OpenCode, Cursor Agent). It can:

- Start interactive agent sessions in `tmux`
- Bridge a **full remote PTY** over WebSocket to your laptop
- Optionally run headless “print” jobs

That is powerful. Treat the API token like a root shell on the machine that runs `agentsd`.

## Supported versions

Only the latest `main` branch is supported until tagged releases exist.

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
   - Store in `/etc/agentsd/env` mode `600` (or your secrets manager)
   - Never commit tokens; rotate if leaked
3. **Workspace allowlist**
   - Keep `[allow].paths` tight
   - Do not set `paths = ["."]` on multi-user hosts unless intentional
4. **Auth on every path**
   - All `/v1/*` routes require bearer token (including WebSocket PTY)
   - Only `/healthz` is public by design
5. **Print jobs (`claude -p`, etc.)**
   - Can use API credits and tool access without a human in the loop
   - Prefer interactive sessions; treat `agentsctl run` as elevated
6. **Agent logins**
   - Subscription/OAuth state lives under the `agentsd` user’s home (often `root` on a dedicated box)
   - Prefer a dedicated non-root user for production multi-tenant scenarios (not fully supported yet)

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security bugs.

Email the maintainer via the address on the GitHub profile for [reloadlife](https://github.com/reloadlife), with:

- Description and impact
- Reproduction steps
- Affected commit / version if known

We will acknowledge when possible and coordinate a fix before disclosure.
