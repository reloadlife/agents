# agents — early design notes (historical)

> **Archive.** Original pre-implementation design for the remote agent control plane.
> Implemented product lives at [github.com/reloadlife/agents](https://github.com/reloadlife/agents).
> Host-specific names and IPs from private drafts have been generalized.

**Status when written:** Design (not implemented)  
**Outcome:** Shipped as `agentsd` / `agentsctl` (TTY-first + optional print jobs)

---

## 1. Problem

SSH works on LAN, but day-to-day agent work needed:

- A clean **job/session API** (submit → status → logs / attach)
- A first-class **CLI** from laptop → agent host
- Safety rails (path allowlist, auth) without a free shell on the public internet

We wanted a **small control plane** on an agent box that:

1. Accepts authenticated remote commands
2. Runs agent CLIs (`claude`, `grok`, `codex`, `opencode`, …) in workspace dirs
3. Streams logs / full PTY and returns useful results
4. Exposes host status without unrestricted shell access

---

## 2. Goals / Non-goals

### Goals

| ID | Goal |
|----|------|
| G1 | Submit work: prompt + cwd + agent (+ caps) |
| G2 | Stream stdout/stderr (SSE) and/or full PTY |
| G3 | Status: disk, load, docker, optional cluster probes |
| G4 | Auth usable from CLI and optional HTTPS edge |
| G5 | Safety: allowlisted paths, elevated caps + confirm |
| G6 | Persist job history under a jobs directory |
| G7 | Coexist with other host tools (chat UIs, etc.) |

### Non-goals (v1)

- Full IDE / remote desktop
- Multi-tenant / multi-user orgs
- Unrestricted remote shell as default
- Auto-merge PRs without confirm
- Many heavy agents in parallel on a small host

---

## 3. Mental model

```
 You (laptop / phone)
        │  HTTPS (Tunnel or Tailscale) or LAN
        ▼
 ┌──────────────────┐
 │  agentsd         │  ← API + session/job supervisor
 │  :8787 localhost │
 └────────┬─────────┘
          │ spawns / supervises
          ▼
 ┌──────────────────┐     ┌─────────────┐
 │ claude / grok /  │────▶│ workspace   │
 │ codex / opencode │     │  repos      │
 └──────────────────┘     └─────────────┘
          │
          ▼
   <jobs_dir>/<id>/   logs, meta.json, result.json
```

### Happy path (coding job — print mode)

1. `agentsctl run -r my-app -a claude "fix the flaky test in src/…"`
2. CLI POSTs with bearer token.
3. Daemon resolves repo → `$workspace_root/my-app`, checks allowlist.
4. Creates job id, runs agent print/exec flags under timeout.
5. Logs stream via SSE; result on exit.

### Happy path (interactive TTY — primary product)

1. `agentsctl session start -a claude --open` (or bare `agentsctl` TUI)
2. Server creates `tmux` session with the interactive CLI.
3. Client attaches full PTY over WebSocket (`/v1/sessions/{id}/pty`).

### Elevated actions

```
agentsctl run -r my-app -a claude "open PR" --cap git_push --cap gh_pr
```

Elevated caps require confirm before the job runs.

---

## 4. Architecture (as designed)

| Component | Role |
|-----------|------|
| **agentsd** | HTTP API + session/job supervisor |
| **agentsctl** | CLI + TUI client |
| **Edge** | Cloudflare Tunnel or Tailscale |
| **Auth v1** | Bearer token |
| **Auth v2** | CF Access JWT and/or Tailscale (future) |

**Listen bind:** `127.0.0.1:8787` only without tunnel/ACL.

### Config sketch (generic)

```toml
listen = "127.0.0.1:8787"
jobs_dir = "/var/lib/agents/jobs"
workspace_root = "/home/YOU/workspace"
max_concurrent_jobs = 1
default_timeout = "30m"
max_timeout = "2h"

[auth]
bearer_env = "AGENTSD_TOKEN"

[agents.claude]
bin = "claude"

[allow]
paths = [
  ".",
  "my-app",
  "team/*",
]

[caps]
default = ["fs_read", "fs_write", "net_install"]
elevated = ["git_push", "gh_pr", "kubectl_write", "shell_raw"]

[status]
gke_context = ""
opendray_url = ""
```

### Directory layout (generic)

```
/usr/local/bin/agentsd
/usr/local/bin/agentsctl
/etc/agentsd/config.toml
/etc/agentsd/env                  # mode 600

~/workspace/
  agents/                         # this git repo (or separate checkout)
  .jobs/
  my-app/
  other-repo/
```

---

## 5. API (v1 sketch)

Auth: `Authorization: Bearer <token>`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Liveness |
| GET | `/v1/status` | Host + queue |
| POST | `/v1/jobs` | Create print job |
| GET | `/v1/jobs/{id}/events` | SSE |
| POST | `/v1/sessions` | Interactive session (implemented) |
| GET | `/v1/sessions/{id}/pty` | WebSocket PTY (implemented) |

---

## 6. CLI sketch

```bash
agentsctl status
agentsctl run -r my-app -a claude "…"
agentsctl session start -a claude --open
agentsctl logs j_… -f
agentsctl cancel j_…
```

---

## 7. Reach

### Path A — Cloudflare Tunnel

```
Internet → CF edge (± Access) → cloudflared → 127.0.0.1:8787
```

### Path B — Tailscale

Serve/ACL to 8787; no public DNS required.

**Never:** raw port-forward of 8787 without auth.

---

## 8. Safety model

1. Path allowlist after symlink resolve  
2. Elevated caps require confirm  
3. Log redaction for common secret patterns  
4. Concurrency limits on small hosts  
5. Timeouts + process group kill on cancel  

---

## 9. Success criteria (original v1)

1. Off-LAN: `agentsctl status` via Tunnel or Tailscale  
2. Job or session against allowlisted cwd succeeds  
3. Queue / concurrency limits work  
4. Cancel works  
5. Invalid path / bad token rejected  
6. systemd restart safe  

---

## 10. Security one-liner

Localhost-only API, Tunnel/Tailscale ingress, bearer (± Access), allowlisted workspaces, capability confirms, redacted logs — **not** a remote root shell.
