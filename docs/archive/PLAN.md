# agents — early implementation plan (historical)

> **Archive.** Companion to [DESIGN.md](./DESIGN.md). Phases below describe the
> original build order; many items shipped in v0.1–v0.2. Host-specific names
> from private drafts have been generalized.

---

## Phase 0 — Skeleton

**Outcome:** Repo builds; `agentsd` serves `/healthz` on localhost.

- [x] Go module + `cmd/agentsd` / `cmd/agentsctl`
- [x] Config load (TOML), Makefile
- [x] Public repo `reloadlife/agents`
- [x] Token via env / `/etc/agentsd/env` or user config

**Demo:** `curl localhost:8787/healthz`

---

## Phase 1 — Jobs core

**Outcome:** Create job, run agent, stream logs, cancel.

- [x] Job store + filesystem layout
- [x] Path resolve + allowlist
- [x] Supervisor: queue, concurrency, timeout
- [x] Agent runners (print/API path)
- [x] SSE events + `agentsctl run --follow`

**Demo:**

```bash
agentsctl run -r my-app -a mock "hello" --follow
```

---

## Phase 2 — Status + hardening

- [x] `/v1/status`
- [x] Elevated caps + confirm
- [x] systemd unit examples
- [x] INSTALL / SECURITY docs

---

## Phase 3 — Remote reach

- [x] Docs for Tunnel / Tailscale / LAN
- [x] Client config (`agentsctl config init`)

**Demo:** Off-LAN `agentsctl status`

---

## Phase 4 — Interactive TTY (became primary)

- [x] tmux sessions
- [x] WebSocket PTY
- [x] TUI session picker
- [x] Workspace picker, PTY reconnect (v0.2)

---

## Phase 5 — Polish (ongoing)

- [x] Non-root install + user systemd unit
- [x] Release tarballs + `scripts/install.sh`
- [x] CI integration (tmux + mock)
- [ ] Homebrew
- [ ] Stronger identity auth (mTLS / Tailscale whois)
- [ ] Multi-user isolation
- [ ] Session recording (privacy docs)

---

## Testing plan

| Level | What |
|-------|------|
| Unit | allowlist, redaction, auth |
| Integration | mock agent + tmux (`-tags=integration`) |
| Manual | real CLI short session on allowlisted cwd |
| Security | no token; path `../../etc`; concurrent jobs |

---

## Install sketch

```bash
curl -fsSL https://raw.githubusercontent.com/reloadlife/agents/main/scripts/install.sh | bash
# or: make build && make install

mkdir -p ~/.config/agentsd
cp config.example.toml ~/.config/agentsd/config.toml
echo "AGENTSD_TOKEN=$(openssl rand -hex 32)" > ~/.config/agentsd/env
chmod 600 ~/.config/agentsd/env
agentsd serve --config ~/.config/agentsd/config.toml
```

See [docs/INSTALL.md](../INSTALL.md) for current steps.
