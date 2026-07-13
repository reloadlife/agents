#!/usr/bin/env bash
# Install agentsd + agentsctl into ~/.local/bin (or PREFIX)
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local}"
REPO="${REPO:-github.com/reloadlife/agents}"
VERSION="${VERSION:-latest}"

need() { command -v "$1" >/dev/null || { echo "need $1" >&2; exit 1; }; }
need go
need git

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

echo "→ cloning ${REPO} (${VERSION})"
if [ "$VERSION" = "latest" ]; then
  git clone --depth 1 "https://${REPO}.git" "$tmpdir/src"
else
  git clone --depth 1 --branch "$VERSION" "https://${REPO}.git" "$tmpdir/src"
fi

cd "$tmpdir/src"
echo "→ building"
go build -ldflags "-s -w" -o agentsd ./cmd/agentsd
go build -ldflags "-s -w" -o agentsctl ./cmd/agentsctl

mkdir -p "$PREFIX/bin"
install -m 755 agentsd agentsctl "$PREFIX/bin/"
echo "✓ installed:"
echo "    $PREFIX/bin/agentsd"
echo "    $PREFIX/bin/agentsctl"
echo
echo "Next:"
echo "  export AGENTSD_TOKEN=\$(openssl rand -hex 32)"
echo "  agentsd serve --config config.example.toml"
echo "  agentsctl config init"
