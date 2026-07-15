# Open-source status

Project: **[reloadlife/agents](https://github.com/reloadlife/agents)**  
Current line: **v0.8.x** (public preview; latest intent **v0.8.11+**)

## Scorecard

| Area | Score | Notes |
|------|-------|--------|
| Core idea | ★★★★★ | Remote PTY for interactive agent CLIs |
| Single-admin host | ★★★★☆ | Works on a dedicated agent box |
| Docs | ★★★★☆ | README, INSTALL, SECURITY, RECORDING, PLAYWRIGHT, WEB |
| Tests | ★★★★☆ | Unit + CI integration (`tmux` + mock session); web build in CI |
| Packaging | ★★★★☆ | Multi-arch release tarballs + `scripts/install.sh` |
| Multi-user security | ★★☆☆☆ | Shared / multi-token bearer — not multi-tenant SaaS |
| Polish | ★★★★☆ | Web UI (projects, git, worktrees), TUI, workspace picker, PTY reconnect |

**Verdict:** Fine for a **personal or small-team agent box**. Do **not** claim production multi-tenant isolation.

## Shipped checklist

- [x] AGPL-3.0 + SECURITY.md + CONTRIBUTING  
- [x] Public repo `github.com/reloadlife/agents`  
- [x] CI (vet, unit, integration, web build, binary build)  
- [x] Tagged releases with `agents_${ver}_${os}_${arch}.tar.gz`  
- [x] Non-root install path (`deploy/agentsd.user.service`, INSTALL.md)  
- [x] Generic examples only (no personal LAN/host configs in tree)  
- [x] Multi-token auth + trusted header (`extra_tokens`, `require_bearer`)  
- [x] Session recording (opt-in) + privacy docs ([RECORDING.md](./RECORDING.md))  
- [x] Worktrees, workspaces create, git API, memory, maps, dashboard  
- [ ] Dogfood v0.8+ for a week without daily breakage  
- [ ] One external install-from-docs success  

## Hygiene

- Never commit real `AGENTSD_TOKEN` values (client/server env gitignored)
- Prefer `listen = "127.0.0.1:8787"` + Tailscale / tunnel; not raw public `0.0.0.0`
- Personal host configs stay **out of git** (use local `config.toml` / `config.local.toml`)
- Leave `sessions.recording` off unless you accept TTY archives on disk ([RECORDING.md](./RECORDING.md))

## Next (not blockers)

1. Dogfood on a real agent host with v0.8.x  
2. Homebrew tap (optional)  
3. Stronger auth options (mTLS, Tailscale whois as primary)  
4. Multi-user isolation (design first)  
5. Recording retention / optional redaction on archive  

## Positioning

> **Remote TTY control plane for AI coding agents**

Interactive subscription UIs on a server; full PTY on your laptop. Not a SaaS, not multi-tenant.
