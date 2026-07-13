# Open-source status

Project: **[reloadlife/agents](https://github.com/reloadlife/agents)**  
Current line: **v0.2.x** (public preview)

## Scorecard

| Area | Score | Notes |
|------|-------|--------|
| Core idea | ★★★★★ | Remote PTY for interactive agent CLIs |
| Single-admin host | ★★★★☆ | Works on a dedicated agent box |
| Docs | ★★★★☆ | README, INSTALL (non-root), SECURITY, PLAYWRIGHT |
| Tests | ★★★★☆ | Unit + CI integration (`tmux` + mock session) |
| Packaging | ★★★★☆ | Multi-arch release tarballs + `scripts/install.sh` |
| Multi-user security | ★★☆☆☆ | Shared bearer token — not multi-tenant SaaS |
| Polish | ★★★★☆ | Default TUI, workspace picker, PTY reconnect |

**Verdict:** Fine for a **personal or small-team agent box**. Do **not** claim production multi-tenant isolation.

## Shipped checklist

- [x] AGPL-3.0 + SECURITY.md + CONTRIBUTING  
- [x] Public repo `github.com/reloadlife/agents`  
- [x] CI (vet, unit, integration, build)  
- [x] Tagged releases with `agents_${ver}_${os}_${arch}.tar.gz`  
- [x] Non-root install path (`deploy/agentsd.user.service`, INSTALL.md)  
- [x] Generic examples only (no personal LAN/host configs in tree)  
- [ ] Dogfood v0.2+ for a week without daily breakage  
- [ ] One external install-from-docs success  

## Hygiene

- Never commit real `AGENTSD_TOKEN` values (client/server env gitignored)
- Prefer `listen = "127.0.0.1:8787"` + Tailscale / tunnel; not raw public `0.0.0.0`
- Personal host configs stay **out of git** (use local `config.toml` / `config.local.toml`)

## Next (not blockers)

1. Dogfood on a real agent host with v0.2.x  
2. Homebrew tap (optional)  
3. Stronger auth options (mTLS, Tailscale whois)  
4. Multi-user isolation (design first)  
5. Session recording (privacy docs required)  

## Positioning

> **Remote TTY control plane for AI coding agents**

Interactive subscription UIs on a server; full PTY on your laptop. Not a SaaS, not multi-tenant.
