# Remote TTY guide

## Model

```text
agentsctl tui / session open
        │  WebSocket + bearer token
        │  binary terminal I/O + resize
        ▼
agentsd  GET /v1/sessions/{id}/pty
        │  PTY
        ▼
tmux attach → interactive agent CLI
```

Interactive CLIs (Claude, Grok, Codex, OpenCode, Cursor) use **subscription/login UIs**, not print/`-p` API modes.

## Client

```bash
agentsctl config init
# set url + token

agentsctl status
agentsctl agents
agentsctl tui
# a / Tab  = cycle agent
# 1-9      = start that agent
# enter    = open session (full PTY)

agentsctl session start -a claude --open
agentsctl session start -a grok --open
agentsctl session open              # latest
agentsctl session kill s_…
```

## Fallback SSH

```bash
agentsctl session open s_… --ssh
```

Requires `ssh_host` in client config and SSH access to the server.

## First-time agent login

On the **server**, as the same user that runs `agentsd`, complete each CLI’s login once:

```bash
claude
grok
codex
opencode
cursor-agent
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| unauthorized | token mismatch; `agentsctl config show` |
| cwd not in allowlist | use `-r <allowed>` or expand server `[allow].paths` |
| agent binary not found | install CLI on server; check `agentsctl agents` |
| blank PTY | resize terminal; check server `tmux` / agent still running |

See [INSTALL.md](INSTALL.md) and [SECURITY.md](../SECURITY.md).
