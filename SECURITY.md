# Security Policy

## What this project is

**agents** (`reloadlife/agents`) is a **remote control plane** for AI coding CLIs (Claude Code, Grok, Codex, OpenCode, Cursor Agent). It can:

- Start interactive agent sessions in `tmux`
- Bridge a **full remote PTY** over WebSocket to your laptop
- Optionally archive session pane snapshots (recording — **default off**)
- Optionally run headless “print” jobs

That is powerful. Treat the API token like a root shell on the machine that runs `agentsd`.

## Supported versions

| Line | Support |
|------|---------|
| **v0.8.x** (current, latest intent **v0.8.11+**) | Security fixes land on `main` and ship in the next patch/minor tag |
| Older (`v0.7.x` and below) | No dedicated backports; upgrade to v0.8.x |

## Threat model (short)

| Asset | Risk if stolen |
|-------|----------------|
| `AGENTSD_TOKEN` / extra tokens | Create/kill sessions, attach PTY, run print jobs, manage server SSH **public** identities, login/switch/logout **gh** accounts (private keys / tokens never returned by the API) |
| PTY WebSocket | Interactive control of the agent TUI (and tools it can run) |
| Embedded web shell (`/`, `/assets/*`) | **Public static SPA only** — no secrets in HTML/JS bundle; all control is via `/v1/*` + token |
| Session recordings | Full terminal scrollback (secrets typed into agents, API keys, code) if `sessions.recording = true` |
| Workspace allowlist escape | Read/write outside intended repos |

## Authentication

### Bearer token (default)

All **`/v1/*`** routes require a valid bearer (including WebSocket PTY).

| Method | How |
|--------|-----|
| HTTP header | `Authorization: Bearer <token>` |
| Query (WebSocket-friendly) | `?token=<token>` on `/v1/sessions/{id}/pty` (and any `/v1/*` if needed) |

Public by design:

- `GET /healthz`
- Non-`/v1` paths (embedded web UI static shell at `/`, `/assets/*`, SPA routes)

The SPA stores the token in **localStorage** and sends the header on REST and `?token=` on the PTY WebSocket. Prefer localhost + Tailscale / tunnel, not a raw public bind.

### Multi-token (`extra_tokens`)

Primary token comes from `auth.bearer_env` (default `AGENTSD_TOKEN`) and is labeled **`default`**.

Additional tokens map **label → env var name** (not the raw secret):

```toml
[auth]
bearer_env = "AGENTSD_TOKEN"
extra_tokens = { ops = "AGENTS_TOKEN_OPS", guest = "AGENTS_TOKEN_GUEST" }
# trusted_header = "Tailscale-User-Login"
# require_bearer = true
```

```bash
export AGENTSD_TOKEN="$(openssl rand -hex 32)"
export AGENTS_TOKEN_OPS="$(openssl rand -hex 32)"
```

Any matching token authenticates. The **label** is the audit actor (optionally combined with a trusted identity header). Labels are for logging / ops distinction only — they do **not** provide multi-tenant isolation.

### Trusted header

```toml
[auth]
trusted_header = "Tailscale-User-Login"   # or Cf-Access-Authenticated-User-Email
require_bearer = true                     # default: header only enriches actor
# require_bearer = false                  # allow header-only auth behind a trusted proxy
```

| `require_bearer` | Behavior |
|------------------|----------|
| `true` (default) | Valid bearer still required; if `trusted_header` is present it is appended to the actor label for audit |
| `false` | A non-empty trusted header alone may authenticate (`proxy:<value>`). **Only** use behind a reverse proxy that strips client-supplied copies of that header |

Never set `require_bearer = false` on a bind reachable without that proxy.

### Token hygiene

- Generate with `openssl rand -hex 32` (≥32 hex chars; short tokens log a soft warning at startup)
- Store env files mode `600` (`~/.config/agentsd/env` or `/etc/agentsd/env`)
- Never commit tokens; rotate if leaked
- Prefer separate `extra_tokens` for different clients (laptop vs CI) so you can rotate one without the other

## Recording privacy (summary)

- **Default off.** Enable only with `[sessions] recording = true`
- Archives pane snapshots under `{jobs_dir}/recordings/` (mode `600` files, `700` dirs)
- Readable by anyone with a valid API token (`GET /v1/recordings`, history search)
- No automatic retention purge — delete on disk or via host backups policy
- Full details: [docs/RECORDING.md](docs/RECORDING.md)

## Hardening checklist

1. **Bind carefully**
   - Prefer `listen = "127.0.0.1:8787"` + Tailscale / SSH tunnel / Cloudflare Tunnel + Access
   - Avoid `0.0.0.0` on untrusted networks
2. **Token**
   - Long random values; mode `600` env files; multi-token via `extra_tokens` when useful
3. **Workspace allowlist**
   - Keep `[allow].paths` tight
   - Do not set `paths = ["."]` on multi-user hosts unless intentional
4. **Auth on every control path**
   - All `/v1/*` require bearer (or trusted-header-only when deliberately configured)
   - Only `/healthz` and the static web shell are public
5. **Recording**
   - Leave off unless you need archives; treat archives as sensitive
6. **Print jobs (`claude -p`, etc.)**
   - Opt-in only; may burn API credits and run non-interactive agent code
7. **Run as non-root when possible**
   - Prefer `deploy/agentsd.user.service` under your own account
   - Units use `KillMode=process` so restart does not kill agent tmux sessions
8. **Playwright / headed browsers**
   - Prefer localhost Playwright server; see [docs/PLAYWRIGHT.md](docs/PLAYWRIGHT.md)

Operational single-admin checklist and lab host pattern: [docs/SECURITY-OPS.md](docs/SECURITY-OPS.md).

## Reporting a vulnerability

Please open a **private** security advisory on GitHub or email the maintainer via the GitHub profile for [reloadlife](https://github.com/reloadlife). Do not file public issues with exploit details until a fix is available.
