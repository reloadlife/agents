# Install & deploy

## From source

```bash
git clone https://github.com/reloadlife/agents.git
cd local-agents
make test && make build
install -m 755 bin/agentsd bin/agentsctl ~/.local/bin/   # or /usr/local/bin
```

Or:

```bash
curl -fsSL https://raw.githubusercontent.com/reloadlife/agents/main/scripts/install.sh | bash
```

## Server config

1. Copy [config.example.toml](../config.example.toml) → `/etc/agentsd/config.toml`
2. Set:
   - `workspace_root` — absolute path to your code tree  
   - `default_cwd` — usually `"."`  
   - `[allow].paths` — repos relative to workspace  
   - `[agents.*.bin]` — CLI names on `PATH`  
3. Token:

```bash
mkdir -p /etc/agentsd
echo "AGENTSD_TOKEN=$(openssl rand -hex 32)" > /etc/agentsd/env
chmod 600 /etc/agentsd/env
```

4. Run:

```bash
set -a; source /etc/agentsd/env; set +a
agentsd serve --config /etc/agentsd/config.toml
```

### systemd

```bash
# edit deploy/agentsd.service paths for your user/layout
cp deploy/agentsd.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now agentsd
```

Prefer a **dedicated user** and `workspace_root` under that user’s home for anything beyond a single-admin lab box.

## Client config

```bash
agentsctl config init
# ~/.config/agentsctl/config.toml
```

```toml
url = "http://127.0.0.1:8787"   # or LAN / Tailscale IP
token = "same-as-server"
# ssh_host only needed for --ssh fallback
```

## Networking patterns

| Pattern | `listen` | Client `url` |
|---------|----------|--------------|
| Same machine | `127.0.0.1:8787` | `http://127.0.0.1:8787` |
| Trusted LAN | `0.0.0.0:8787` | `http://server-lan-ip:8787` |
| Tailscale | `127.0.0.1` or tailnet IP | `http://100.x.y.z:8787` |
| Cloudflare Tunnel | `127.0.0.1:8787` | `https://agents-api.example.com` + Access |

## Verify

```bash
curl -sS http://SERVER:8787/healthz
agentsctl status
agentsctl agents
agentsctl session start -a claude --open
```
