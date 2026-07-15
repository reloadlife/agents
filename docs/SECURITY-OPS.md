# Security ops — single-admin host

Short operational checklist for running **agentsd** as a personal or lab
control plane. Not multi-tenant isolation.

## Single-admin checklist

| Item | Recommendation |
|------|----------------|
| **Listen** | `listen = "127.0.0.1:8787"` — reach via Tailscale, SSH tunnel, or Cloudflare Tunnel + Access. Avoid raw `0.0.0.0` on untrusted nets. |
| **Token file** | `AGENTSD_TOKEN` in `~/.config/agentsd/env` or `/etc/agentsd/env`, mode **`600`**. Generate with `openssl rand -hex 32`. |
| **Allowlist** | Keep `[allow].paths` to repos you intend agents to touch. Avoid `"."` on shared hosts. |
| **Recording** | Leave **`sessions.recording` off** unless you need archives. See [RECORDING.md](./RECORDING.md). |
| **Playwright** | Prefer localhost (`ws://127.0.0.1:…`); do not expose headed browser debug ports publicly. See [PLAYWRIGHT.md](./PLAYWRIGHT.md). |
| **systemd** | Use units with **`KillMode=process`** so `restart` does not kill the tmux server and every agent. Both `deploy/agentsd.user.service` and `deploy/agentsd.service` set this. |
| **User** | Prefer non-root: `deploy/agentsd.user.service` + `~/.local/bin`. |
| **Web UI** | Public static shell is OK; all control is `/v1/*` + token. Disable with `[web] enabled = false` for API-only. |
| **Multi-token** | Optional `extra_tokens` for laptop vs CI; rotate independently. Still not multi-user isolation. |
| **Trusted header** | Only with a proxy that strips spoofed headers; keep `require_bearer = true` unless you fully trust the proxy path. |

## Lab host pattern

Typical dedicated “agent box” on a trusted LAN / Tailnet:

```text
  laptop  ──Tailscale──►  agents host (127.0.0.1:8787)
                              │
                              ├── agentsd (user systemd, KillMode=process)
                              ├── tmux agents (claude/grok/…)
                              ├── workspace_root under $HOME
                              └── optional Xvfb :99 + Playwright on localhost
```

1. Install non-root (`make install` or `scripts/install.sh` → `~/.local/bin`).  
2. Config: `~/.config/agentsd/config.toml` from `config.example.toml`.  
3. Env: `chmod 600 ~/.config/agentsd/env` with a long token.  
4. `systemctl --user enable --now agentsd` (+ `loginctl enable-linger` if needed).  
5. Client: `agentsctl` URL `http://100.x.y.z:8787` (tailnet) or tunnel URL; same token.  
6. Doctor/smoke: `curl -sS http://127.0.0.1:8787/healthz` and `agentsctl status`.

System-wide alternative: binaries in `/usr/local/bin`, config `/etc/agentsd/`, unit
`deploy/agentsd.service` with a dedicated user — see [INSTALL.md](./INSTALL.md).

## After compromise or leak

1. Rotate all tokens (`AGENTSD_TOKEN` + every `extra_tokens` env).  
2. Restart agentsd so old tokens stop working.  
3. Review `jobs_dir` sessions/recordings and audit log if enabled.  
4. Revoke gh / agent account credentials that may have been used on the host.  
5. Re-check listen address and firewall / Tailscale ACLs.

## Related

- [SECURITY.md](../SECURITY.md) — threat model and auth details  
- [RECORDING.md](./RECORDING.md) — recording privacy  
- [INSTALL.md](./INSTALL.md) — install paths  
- [ARCHITECTURE.md](./ARCHITECTURE.md) — trust boundaries  
