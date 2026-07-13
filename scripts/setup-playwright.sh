#!/usr/bin/env bash
# Install Playwright browsers + OS deps and (optionally) enable Xvfb for headed mode.
set -euo pipefail

echo "→ Playwright system deps (requires root for apt)"
if command -v npx >/dev/null; then
  if [ "$(id -u)" -eq 0 ]; then
    npx --yes playwright@1.61.1 install-deps chromium 2>/dev/null \
      || npx --yes playwright install-deps chromium || true
  else
    echo "  (skip install-deps — re-run as root for apt packages)"
  fi
  echo "→ Playwright browsers (user cache)"
  npx --yes playwright@1.61.1 install chromium
else
  echo "npx not found; install Node.js first" >&2
  exit 1
fi

if command -v Xvfb >/dev/null; then
  echo "→ Xvfb present"
else
  echo "→ installing xvfb"
  if [ "$(id -u)" -eq 0 ]; then
    apt-get update -qq && apt-get install -y -qq xvfb
  else
    echo "  install xvfb manually: sudo apt install xvfb"
  fi
fi

echo
echo "Enable virtual display:"
echo "  sudo cp deploy/xvfb.service /etc/systemd/system/"
echo "  sudo systemctl enable --now xvfb"
echo
echo "In /etc/agentsd/config.toml:"
echo "  [sessions]"
echo "  display = \":99\""
echo "  # optional remote server:"
echo "  # playwright_server = \"ws://127.0.0.1:9333\""
echo
echo "Optional container:"
echo "  docker compose -f deploy/docker-compose.playwright.yml up -d"
echo
echo "Done. Restart agentsd after config change."
