# agents-remote — Implementation Plan

Companion to [DESIGN.md](./DESIGN.md). Ordered for vertical slices you can demo.

---

## Phase 0 — Skeleton (half day)

**Outcome:** Repo builds; `agentsd` serves `/healthz` on localhost; systemd unit optional.

- [ ] `mkdir ~/workspace/agents-remote` (done) + `go mod init`
- [ ] `cmd/agentsd` — HTTP server, config load (TOML)
- [ ] `cmd/agentsctl` — `version`, `status` stub
- [ ] Makefile: `build`, `install`, `run`
- [ ] Private git repo `reloadlife/agents-remote`
- [ ] Generate `AGENTSD_TOKEN` → `secret set env/agentsd.sops.env`
- [ ] Install token to `/etc/agentsd/env` (600) via small install script

**Demo:** `curl -H "Authorization: Bearer …" localhost:8787/healthz`

---

## Phase 1 — Jobs core (1–2 days)

**Outcome:** Create job, run real agent, stream logs, cancel.

- [ ] Job store: sqlite schema (`jobs` table) + filesystem `jobs_dir/<id>/`
- [ ] Path resolve + allowlist
- [ ] Supervisor: queue, max concurrent=1, process group, timeout
- [ ] Agent runners: start with `claude -p` (prompt); add grok/codex/opencode adapters
- [ ] API: POST/GET jobs, GET log, GET events (SSE), POST cancel
- [ ] State machine + reboot recovery (running → interrupted)
- [ ] Log redaction middleware
- [ ] `agentsctl run --follow`, `logs`, `cancel`, `jobs`

**Demo:** From agents localhost:

```bash
agentsctl run -r DollarChande -a claude "Reply with: hostname, pwd, go version" --follow
```

---

## Phase 2 — Status + hardening (half day)

**Outcome:** Ops dashboard via CLI.

- [ ] `/v1/status`: load, mem, disk, docker systemd, opendray, kubectl nodes
- [ ] Elevated caps + `awaiting_confirm` + `agentsctl confirm`
- [ ] Refuse jobs if disk full / agent binary missing
- [ ] systemd unit `agentsd.service` enabled
- [ ] `docs/RUNBOOK.md`

**Demo:** `agentsctl status` shows GKE 4/4 Ready

---

## Phase 3 — Remote reach (half–1 day)

**Outcome:** Works off LAN.

### 3a Cloudflare Tunnel (default)

- [ ] Install `cloudflared` on agents
- [ ] Create tunnel + DNS `agents-api.<your-domain>`
- [ ] Ingress → `http://127.0.0.1:8787`
- [ ] (Optional) CF Access email policy
- [ ] Mac `~/.config/agentsctl/config.toml` → public URL + token
- [ ] Verify from phone network (cellular)

### 3b Tailscale alternative

- [ ] Install tailscale on agents
- [ ] ACL / serve 8787
- [ ] CLI base_url = MagicDNS name

**Demo:** On Mac outside home Wi‑Fi: `agentsctl status && agentsctl run … --follow`

---

## Phase 4 — Telegram thin client (1 day)

**Outcome:** Phone mission control without OpenDray dependency.

- [ ] Bot token in sops
- [ ] Allowlist user id
- [ ] Commands: status, run, jobs, log, cancel, confirm
- [ ] Prefer calling localhost API (bot runs on agents)

**Demo:** Telegram `/status` and `/run DollarChande claude hostname`

---

## Phase 5 — Polish (ongoing)

- [ ] Artifact capture (git diff stat into result.json)
- [ ] Webhook notify (optional)
- [ ] `direnv exec` when `.envrc` present
- [ ] Multi-job concurrency profile “light” vs “heavy”
- [ ] OpenDray note/link to job id
- [ ] Metrics / simple HTML status page (auth’d)

---

## Build order (PRs / commits)

| Slice | Delivers |
|-------|----------|
| PR1 | Skeleton server + config + auth middleware |
| PR2 | Job store + runner + SSE + CLI run/follow |
| PR3 | Status + systemd + runbook |
| PR4 | Tunnel install docs + agentsctl remote config |
| PR5 | Telegram bot |

---

## Testing plan

| Level | What |
|-------|------|
| Unit | allowlist paths, redaction, state transitions |
| Integration | mock agent binary (`/bin/echo` / script) end-to-end |
| Manual | real `claude -p` short prompt on DollarChande |
| Security | curl without token; path `../../etc`; concurrent jobs |

Mock agent for CI:

```bash
# test agent
#!/bin/sh
echo "hello from mock"
exit 0
```

Config points `agents.mock.bin` at that script.

---

## Install sketch (agents host)

```bash
cd ~/workspace/agents-remote
make build
sudo make install   # binaries + systemd + /etc/agentsd

# token
TOKEN=$(openssl rand -hex 32)
secret set env/agentsd.sops.env AGENTSD_TOKEN "$TOKEN"
# materialize for systemd:
secret export env/agentsd.sops.env | grep AGENTSD_TOKEN | sudo tee /etc/agentsd/env
sudo chmod 600 /etc/agentsd/env
sudo systemctl enable --now agentsd

agentsctl -c /root/.config/agentsctl/config.toml status
```

---

## Runbook outline (to write in phase 2)

1. Start/stop/restart agentsd  
2. Where logs live (`journalctl -u agentsd`, `.jobs/`)  
3. Rotate token  
4. Tunnel down troubleshooting  
5. Job stuck / cancel / mark failed  
6. Disk cleanup of old jobs  

---

## Effort estimate

| Phase | Time | Dependency |
|-------|------|------------|
| 0 Skeleton | 0.5 d | — |
| 1 Jobs | 1–2 d | — |
| 2 Status/systemd | 0.5 d | Phase 1 |
| 3 Tunnel | 0.5–1 d | Domain + CF account |
| 4 Telegram | 1 d | BotFather token |
| **MVP usable remotely** | **~3–4 days** | Phases 0–3 |

---

## What you do vs what we automate

| You | We (implementation) |
|-----|---------------------|
| Confirm domain name for Tunnel | Code, unit, systemd |
| CF account / Access policy click | cloudflared config templates |
| Telegram bot token (phase 4) | Bot wiring |
| First real agent policy (permissions) | Runner flags research per CLI |

---

## Next step after plan approval

1. Lock open decisions (hostname, Tunnel vs Tailscale)  
2. Scaffold Go module + PR1  
3. Vertical slice until localhost `agentsctl run --follow` works  
4. Then Tunnel  

Do **not** start Tunnel before localhost jobs work.
