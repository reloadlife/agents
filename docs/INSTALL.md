# Install & deploy

## From source

```bash
git clone https://github.com/reloadlife/agents.git
cd agents   # or local-agents
make test && make build
make install   # → ~/.local/bin (non-root)
# or: sudo install -m 755 bin/agentsd bin/agentsctl /usr/local/bin/
```

Or release tarballs (from GitHub Releases):

```bash
# agents_v0.2.0_linux_amd64.tar.gz  (also local-agents_* alias)
tar xzf agents_v*.tar.gz
install -m 755 agents_*/agentsd agents_*/agentsctl ~/.local/bin/
```

Or:

```bash
curl -fsSL https://raw.githubusercontent.com/reloadlife/agents/main/scripts/install.sh | bash
```

## Non-root install (recommended)

Run `agentsd` as your own user with a **systemd user unit** — no root daemon, configs under `$HOME`.

```bash
make build && make install   # ~/.local/bin

mkdir -p ~/.config/agentsd ~/.local/share/agents
cp config.example.toml ~/.config/agentsd/config.toml
# edit workspace_root, allow.paths, agents.*

umask 077
echo "AGENTSD_TOKEN=$(openssl rand -hex 32)" > ~/.config/agentsd/env
chmod 600 ~/.config/agentsd/env

# point jobs_dir at a writable path, e.g. in config.toml:
#   jobs_dir = "/home/YOU/.local/share/agents/jobs"
#   workspace_root = "/home/YOU/work"

cp deploy/agentsd.user.service ~/.config/systemd/user/agentsd.service
systemctl --user daemon-reload
systemctl --user enable --now agentsd
# optional: survive logout
loginctl enable-linger "$USER"

# client (same machine or remote)
agentsctl config init
# url = "http://127.0.0.1:8787"
# token = <same as AGENTSD_TOKEN>
agentsctl status
agentsctl            # opens TUI by default
```

See [deploy/agentsd.user.service](../deploy/agentsd.user.service).

## Server config (system-wide)

1. Copy [config.example.toml](../config.example.toml) → `/etc/agentsd/config.toml`
2. Set:
   - `workspace_root` — absolute path to your code tree  
   - `default_cwd` — usually `"."`  
   - `[allow].paths` — repos relative to workspace  
   - `[agents.*.bin]` — CLI names on `PATH`  
3. Token:

```bash
sudo mkdir -p /etc/agentsd
echo "AGENTSD_TOKEN=$(openssl rand -hex 32)" | sudo tee /etc/agentsd/env
sudo chmod 600 /etc/agentsd/env
```

4. Run:

```bash
set -a; source /etc/agentsd/env; set +a
agentsd serve --config /etc/agentsd/config.toml
```

### systemd (system unit)

```bash
# prefer User= in deploy/agentsd.service rather than root
sudo cp deploy/agentsd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now agentsd
```

Prefer a **dedicated user** (or the non-root user unit above) and `workspace_root` under that user’s home for anything beyond a single-admin lab box.
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
