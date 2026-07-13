# Open-source readiness

## Honest scorecard (now)

| Area | Score | Notes |
|------|-------|--------|
| Core idea | ★★★★★ | Clear, useful, differentiated (remote PTY for agent CLIs) |
| Works for single-admin host | ★★★★☆ | Proven on a dedicated box |
| Docs for strangers | ★★★★☆ | README / INSTALL / SECURITY after overhaul |
| Tests | ★★★★☆ | Auth, pathallow, redact; CI integration (tmux + mock session) |
| Packaging | ★★★☆☆ | CI + tag release workflow; no apt/brew yet |
| Multi-user security | ★★☆☆☆ | Shared token, often root — document, don’t claim SaaS-ready |
| Polish | ★★★☆☆ | CLI/TUI good enough for v0.1 |

**Verdict:** Ready for a **public `v0.1.0` preview** (“works for a personal/team agent box”), **not** for “production multi-tenant platform” claims.

## Before you press publish

1. **Repo**
   - Create `github.com/reloadlife/agents` (private → public when ready)
   - First commit of this tree
   - Push `main`
2. **Secrets hygiene**
   - Ensure no real tokens in git history (client configs are gitignored)
   - Rotate `AGENTSD_TOKEN` if it ever appeared in chat logs
3. **Tag**
   - `git tag v0.1.0 && git push --tags` → release workflow builds multi-arch tarballs
4. **README social proof**
   - Screenshots of `agentsctl status` / `tui` (optional)
   - Short demo GIF of PTY attach
5. **Positioning**
   - Title: *Remote TTY control plane for AI coding agents*
   - Avoid overclaiming isolation / multi-tenancy

## What “good enough” means

Ship when:

- [x] AGPL-3.0 license  
- [x] Security policy  
- [x] CI green on PR  
- [x] Install docs without your LAN IPs as the only story  
- [x] `make test && make build` works on a clean clone  
- [ ] You’ve used it yourself for a week without daily breakage  
- [ ] One external friend can install from docs alone  

## What to improve after v0.1

- Non-root packaging defaults  
- ~~Integration test with `tmux` + mock agent in CI~~ (done in v0.2.0) 
- Homebrew / `go install` one-liners  
- Optional stronger auth (mTLS, Tailscale whois)  
