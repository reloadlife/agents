# Headed browsers & Playwright

Agent sessions are **interactive TTYs**. For browser automation they also need a **display** so Chromium can run non-headless (required by many Playwright flows and agent browser tools).

## Architecture

```text
agent session (tmux)
  DISPLAY=:99
  PLAYWRIGHT_HEADLESS=0
        │
        ▼
   Xvfb :99  (virtual 1920x1080 screen)
        │
        ▼
   Chromium / Playwright (headed)
```

Optional: Playwright **server container** for a stable browser process agents connect to via WebSocket.

## Server setup

```bash
# from repo root on the agent machine
sudo bash scripts/setup-playwright.sh

sudo cp deploy/xvfb.service /etc/systemd/system/
sudo systemctl enable --now xvfb
```

`agentsd` config (`/etc/agentsd/config.toml`):

```toml
[sessions]
display = ":99"
# optional: path to browser cache (default ~/.cache/ms-playwright)
# playwright_browsers_path = "/root/.cache/ms-playwright"
# optional: connect to containerized Playwright server
# playwright_server = "ws://127.0.0.1:9333"

# arbitrary extra env for every agent session
# [sessions.env]
# MY_FLAG = "1"
```

Restart:

```bash
sudo systemctl restart agentsd
```

Sessions inherit:

| Env | Purpose |
|-----|---------|
| `DISPLAY` | e.g. `:99` for Xvfb |
| `PLAYWRIGHT_HEADLESS=0` | prefer headed |
| `HEADED=1` | convention for some tools |
| `PLAYWRIGHT_BROWSERS_PATH` | browser binary cache |
| `PLAYWRIGHT_SERVER` / `PW_TEST_SERVER` | remote Playwright WS (if configured) |
| `PLAYWRIGHT_CHROMIUM_SANDBOX=0` | often required as root / in VMs |

## Optional Playwright container

```bash
# Xvfb must be up on the host
sudo systemctl start xvfb
docker compose -f deploy/docker-compose.playwright.yml up -d
```

Then set `sessions.playwright_server = "ws://127.0.0.1:9333"` and restart `agentsd`.

In code / agent tools:

```js
const { chromium } = require("playwright");
const browser = await chromium.connect(process.env.PLAYWRIGHT_SERVER);
// or local headed against Xvfb:
// const browser = await chromium.launch({ headless: false });
```

## Manage with agentsctl

```bash
agentsctl playwright status      # display, xvfb, container, server port
agentsctl playwright start       # systemctl start xvfb + docker compose up -d
agentsctl playwright stop        # stop container (leaves Xvfb)
agentsctl playwright restart
agentsctl playwright install     # npx playwright install chromium on server
```

API (same token as other routes):

| Method | Path |
|--------|------|
| GET | `/v1/playwright` |
| POST | `/v1/playwright/start` |
| POST | `/v1/playwright/stop` |
| POST | `/v1/playwright/restart` |
| POST | `/v1/playwright/install` |

## Verify

```bash
agentsctl playwright status
agentsctl playwright start

# headed launch on server (should not error)
DISPLAY=:99 npx playwright@1.61.1 open about:blank

# from agentsctl session — agent should see DISPLAY
agentsctl session start -a claude --open
# then in agent: run `echo $DISPLAY` → :99
```

## Notes

- This is a **virtual** display (no physical monitor). “Non-headless” means full browser stack + windowing, not that you see a screen unless you VNC/stream `:99`.
- To **view** the display: `x11vnc -display :99` + VNC client, or similar (not bundled).
- Print/API jobs (`agentsctl run`) get a trimmed env; DISPLAY is included when set in the daemon environment.
