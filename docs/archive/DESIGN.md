# agents-remote — Design & Operating Model

> Remote control plane for the **agents** host (`192.168.20.6`).
> Submit agent jobs, stream progress, inspect host/GKE health — from phone, Mac CLI, or Telegram.

**Status:** Design (not implemented)  
**Owner:** Mamad  
**Host:** Debian agents VM · ~8.5 GiB RAM · dual GitHub · GKE bridgehq · sops/age secrets  

---

## 1. Problem

SSH/Zed work on LAN. OpenDray gives Telegram + web on `:8770`, but:

- No clean **job API** (submit → status → logs → result)
- No first-class **CLI** for Mac → agents
- OpenDray couples UI + providers; hard to extend safely
- Outside-home access is either raw LAN or ad-hoc

We want a **small, owned control plane** on the agents box that:

1. Accepts authenticated remote commands
2. Runs agent CLIs (`claude`, `grok`, `codex`, `opencode`) in workspace dirs
3. Streams logs and returns artifacts (diff summary, PR URL, exit code)
4. Exposes host/GKE status without giving a free shell to the internet

---

## 2. Goals / Non-goals

### Goals

| ID | Goal |
|----|------|
| G1 | Submit async jobs: prompt + cwd + agent + budget |
| G2 | Stream stdout/stderr (SSE) |
| G3 | Status endpoint: disk, load, docker, opendray, kubectl nodes |
| G4 | Auth that works from CLI, Telegram, and optional public HTTPS |
| G5 | Hard safety rails: allowlisted paths, command classes, confirm for danger |
| G6 | Persist job history under `~/workspace/.jobs/` |
| G7 | Coexist with OpenDray (do not replace day one) |

### Non-goals (v1)

- Full IDE / remote desktop
- Multi-tenant / multi-user orgs
- Replacing OpenDray session multiplexer
- Unrestricted remote shell as default
- Auto-merge PRs or auto-prod deploy without confirm
- Running many heavy agents in parallel (RAM-bound)

---

## 3. How it is supposed to work (mental model)

Think of the agents machine as a **worker farm of one**:

```
 You (phone / Mac / Telegram)
        │  HTTPS (Tunnel or Tailscale)
        ▼
 ┌──────────────────┐
 │  agents-remote   │  ← thin API + job supervisor (this project)
 │  :8787 localhost │
 └────────┬─────────┘
          │ spawns / supervises
          ▼
 ┌──────────────────┐     ┌─────────────┐
 │ claude / grok /  │────▶│ ~/workspace │
 │ codex / opencode │     │  repos      │
 └──────────────────┘     └─────────────┘
          │
          ▼
   ~/.jobs/<id>/   logs, meta.json, result.json
```

### Happy path (coding job)

1. From Mac:
   `agentsctl run -r DollarChande -a claude "fix the flaky test in src/…"`
2. CLI POSTs to `agents-remote` with bearer token (from sops / keychain).
3. Daemon validates auth, resolves repo → `/root/workspace/DollarChande`, checks allowlist.
4. Creates job id `j_01H…`, writes `meta.json`, starts process under a job session:
   ```
   cd /root/workspace/DollarChande
   claude -p "…"    # exact flags finalized in impl per agent
   ```
5. stdout/stderr tee into `~/workspace/.jobs/<id>/log.txt` and stream to client (SSE).
6. On exit: `result.json` with exit code, duration, optional summary.
7. CLI prints summary; if Telegram linked, bot can notify the same job id.

### Happy path (status)

```
agentsctl status
→ load 0.4 · disk 29% · docker up · opendray active · GKE 4/4 Ready
```

### Happy path (dangerous action)

```
agentsctl run -r mamad-core -a claude "open PR and merge" --cap git_push --cap gh_pr
```

Daemon classifies elevated caps (`git_push`, `gh_pr`, `kubectl_write`):

- Job is created in state `awaiting_confirm`
- You get a one-time confirm step (CLI prompt or Telegram inline button)
- Only after confirm does the job enter `queued` → `running`
- v1 alternative: agent runs local-only and stops before push, posting a plan for confirm

**v1 ships simple mode:** elevated caps always require confirm before start.

### How OpenDray fits

| Layer | Role |
|-------|------|
| **agents-remote** | Job API, CLI, status, safety, history |
| **OpenDray** | Optional chat UI + existing Telegram channel |
| Later | Thin Telegram bot → agents-remote only |

Day one: **parallel**. Do not break OpenDray.

---

## 4. Architecture

### 4.1 Components

| Component | Where | Role |
|-----------|--------|------|
| **agentsd** | agents host systemd | HTTP API + job supervisor |
| **agentsctl** | Mac + agents PATH | CLI client |
| **Edge reach** | CF Tunnel *or* Tailscale | Path to API |
| **Telegram bot** (phase 2) | sidecar or agentsd plugin | Phone UX |
| **secrets** | age/sops `~/secrets` | API tokens, bot token |
| **jobs store** | `~/workspace/.jobs/` + sqlite | Durable history |

### 4.2 Recommended stack

| Choice | Decision | Why |
|--------|----------|-----|
| Language | **Go** | On host, static binary, solid process control |
| HTTP | stdlib `net/http` (+ small router if needed) | Tiny deps |
| Job IDs | ULID / UUIDv7 | Sortable |
| Stream | **SSE** first | CLI/`curl -N` friendly |
| DB | **sqlite** + log files | Survive restarts |
| Auth v1 | Bearer token (sops) | Fast |
| Auth v2 | CF Access JWT and/or Tailscale | Stronger |
| Reach default | **Cloudflare Tunnel** | Phone-friendly HTTPS |
| Alt reach | Tailscale Serve | Zero public surface |

**Listen bind:** `127.0.0.1:8787` only. Never public bind without tunnel/ACL.

### 4.3 Process model

```
systemd: agentsd.service
  EnvironmentFile: /etc/agentsd/env   # AGENTSD_TOKEN from sops install step
  ExecStart: /usr/local/bin/agentsd serve --config /etc/agentsd/config.toml

Per job:
  - context with timeout (default 30m, max 2h)
  - process group (kill children on cancel)
  - cleaned env + optional secret injection by cap
  - cwd: resolved workspace path
  - concurrency: global max 1 heavy agent (configurable; 8GB RAM)
```

**Queue:** if slot full → state `queued`, FIFO.

### 4.4 Directory layout (on agents)

```
/usr/local/bin/agentsd
/usr/local/bin/agentsctl

/etc/agentsd/config.toml
/etc/agentsd/env                  # mode 600

~/workspace/
  agents-remote/                  # git repo
    cmd/agentsd/
    cmd/agentsctl/
    internal/
      api/
      job/
      auth/
      status/
      agent/
    docs/DESIGN.md
    docs/PLAN.md
    docs/RUNBOOK.md
  .jobs/
    jobs.db
    j_01HXXX/
      meta.json
      log.txt
      result.json
      artifacts/
  DollarChande/
  reloadlife/...
  mamaru/...
```

### 4.5 Config sketch

```toml
listen = "127.0.0.1:8787"
jobs_dir = "/root/workspace/.jobs"
workspace_root = "/root/workspace"
max_concurrent_jobs = 1
default_timeout = "30m"
max_timeout = "2h"

[auth]
bearer_env = "AGENTSD_TOKEN"

[agents.claude]
bin = "claude"
[agents.grok]
bin = "grok"
[agents.codex]
bin = "codex"
[agents.opencode]
bin = "opencode"

[allow]
paths = [
  "DollarChande",
  "reloadlife/*",
  "mamaru/*",
  "agents-remote",
]

[caps]
default = ["fs_read", "fs_write", "net_install"]
elevated = ["git_push", "gh_pr", "kubectl_write", "shell_raw"]

[status]
gke_context = "gke_bridgehq-482419_europe-west1-b_bridgehq-prod"
opendray_url = "http://127.0.0.1:8770"
```

---

## 5. API (v1)

Base: `https://agents-api.<domain>` or Tailscale MagicDNS.

Auth: `Authorization: Bearer <token>`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Tunnel health (no secrets) |
| GET | `/v1/status` | Host + GKE + queue |
| POST | `/v1/jobs` | Create job |
| GET | `/v1/jobs` | List jobs |
| GET | `/v1/jobs/{id}` | Job detail |
| GET | `/v1/jobs/{id}/events` | SSE stream |
| GET | `/v1/jobs/{id}/log` | Full log text |
| POST | `/v1/jobs/{id}/cancel` | Cancel |
| POST | `/v1/jobs/{id}/confirm` | Elevated confirm |

### POST /v1/jobs body

```json
{
  "prompt": "fix flaky test",
  "agent": "claude",
  "cwd": "DollarChande",
  "timeout": "45m",
  "caps": [],
  "title": "optional label"
}
```

### Job states

```
queued → running → succeeded | failed | cancelled
                ↘ awaiting_confirm → queued (after confirm)
```

### SSE events

```
event: log
data: {"line":"..."}

event: state
data: {"state":"running"}

event: result
data: {"exit_code":0,"summary":"..."}
```

---

## 6. CLI (`agentsctl`)

```bash
# ~/.config/agentsctl/config.toml  → base_url + token path

agentsctl status
agentsctl jobs
agentsctl run -r DollarChande -a claude "…"
agentsctl run -r reloadlife/mamad-core -a grok "…" --follow
agentsctl logs j_01H... -f
agentsctl cancel j_01H...
agentsctl confirm j_01H...
```

---

## 7. Auth & reach

### Path A — Cloudflare Tunnel (default)

```
Internet → CF edge (± Access) → cloudflared on agents → 127.0.0.1:8787
```

1. `cloudflared` on agents  
2. Hostname e.g. `agents-api.mamad.dev` → localhost:8787  
3. Bearer token in sops (`env/agentsd.sops.env`)  
4. Prefer CF Access (your email) when domain ready  

### Path B — Tailscale

1. tailscaled on agents + clients  
2. Serve/ACL to 8787  
3. No public DNS  

**Never:** raw router port-forward of 8787/8770 without auth.

---

## 8. Safety model

1. **Path allowlist** — cwd under workspace_root after symlink resolve  
2. **Caps** — elevated requires confirm  
3. **Non-interactive agents** — headless flags only  
4. **Clean env** — no full root environment to children  
5. **Log redaction** — scrub `gho_`, `sk-`, `ghp_`, private keys patterns  
6. **Concurrency 1** — protect 8GB host  
7. **Timeouts + process group kill** on cancel  

---

## 9. Telegram (phase 2)

| Command | Action |
|---------|--------|
| `/status` | GET /v1/status |
| `/run <repo> <agent> <prompt>` | create job |
| `/jobs` | list |
| `/log <id>` | tail |
| `/cancel <id>` | cancel |
| `/confirm <id>` | elevated confirm |

Allowlist: your Telegram user id only.

---

## 10. Failure modes

| Failure | Behavior |
|---------|----------|
| Agent binary missing | job failed, clear error |
| cwd not allowlisted | HTTP 400 |
| Bad token | HTTP 401 |
| Host reboot mid-job | mark running → interrupted |
| Tunnel down | SSH/Tailscale still work |
| Disk > 95% | refuse new jobs |

---

## 11. Success criteria (v1 done)

1. Off-LAN: `agentsctl status` via Tunnel  
2. `agentsctl run -r DollarChande -a claude "print hostname and pwd"` succeeds  
3. Second job queues (no parallel stampede)  
4. Cancel works  
5. Invalid path / bad token rejected  
6. systemd restart safe  
7. RUNBOOK.md written  

---

## 12. Open decisions

| # | Decision | Default |
|---|----------|---------|
| D1 | Reach | **CF Tunnel** |
| D2 | Hostname | `agents-api.mamad.dev` (confirm) |
| D3 | CF Access | Prefer yes |
| D4 | Default agent | `claude` |
| D5 | Language | **Go** |
| D6 | GitHub repo | `reloadlife/agents-remote` private |

---

## 13. Security one-liner

Localhost-only API, Tunnel/Tailscale ingress, bearer (± CF Access), allowlisted workspaces, capability confirms, redacted logs — **not** a remote root shell.
