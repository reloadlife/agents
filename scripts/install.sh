#!/usr/bin/env bash
# Install agentsd + agentsctl into PREFIX (default: ~/.local)
#
# Prefer prebuilt GitHub release tarballs. Fall back to building from source
# when SOURCE=1 or when curl/download fails and Go is available.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/reloadlife/agents/main/scripts/install.sh | bash
#   VERSION=v0.2.0 bash scripts/install.sh
#   PREFIX=/usr/local sudo bash scripts/install.sh
#   SOURCE=1 bash scripts/install.sh          # force go build from git
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local}"
REPO="${REPO:-reloadlife/agents}"
VERSION="${VERSION:-latest}"   # latest | v0.2.0 | ...
SOURCE="${SOURCE:-0}"
BIN_DIR="${PREFIX}/bin"

need() { command -v "$1" >/dev/null 2>&1 || { echo "need $1" >&2; exit 1; }; }

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "unsupported arch: $arch" >&2
    exit 1
    ;;
esac
case "$os" in
  linux|darwin) ;;
  *)
    echo "unsupported OS: $os (want linux|darwin)" >&2
    exit 1
    ;;
esac

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

resolve_tag() {
  if [ "$VERSION" != "latest" ]; then
    echo "$VERSION"
    return
  fi
  # GitHub latest-release redirect
  need curl
  local loc
  loc=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" || true)
  # url ends with /tag/vX.Y.Z
  echo "${loc##*/}"
}

install_bins() {
  local src_dir="$1"
  mkdir -p "$BIN_DIR"
  install -m 755 "$src_dir/agentsd" "$src_dir/agentsctl" "$BIN_DIR/"
  echo "✓ installed:"
  echo "    $BIN_DIR/agentsd"
  echo "    $BIN_DIR/agentsctl"
}

install_from_release() {
  need curl
  need tar
  local tag
  tag=$(resolve_tag)
  if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
    echo "could not resolve latest release tag" >&2
    return 1
  fi
  # Prefer agents_*; fall back to legacy local-agents_*
  local names=(
    "agents_${tag}_${os}_${arch}.tar.gz"
    "local-agents_${tag}_${os}_${arch}.tar.gz"
  )
  local url asset
  for asset in "${names[@]}"; do
    url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
    echo "→ trying $url"
    if curl -fsSL "$url" -o "$tmpdir/release.tar.gz"; then
      tar -C "$tmpdir" -xzf "$tmpdir/release.tar.gz"
      # tarball contains a single top-level dir
      local dir
      dir=$(find "$tmpdir" -maxdepth 1 -type d ! -path "$tmpdir" | head -1)
      if [ -z "$dir" ] || [ ! -x "$dir/agentsd" ]; then
        echo "unexpected archive layout" >&2
        return 1
      fi
      install_bins "$dir"
      echo
      echo "version: $tag"
      echo "Next (server):"
      echo "  mkdir -p ~/.config/agentsd && cp config.example.toml ~/.config/agentsd/config.toml  # from repo or tarball"
      echo "  echo \"AGENTSD_TOKEN=\$(openssl rand -hex 32)\" > ~/.config/agentsd/env && chmod 600 ~/.config/agentsd/env"
      echo "  agentsd serve --config ~/.config/agentsd/config.toml"
      echo "Next (client):"
      echo "  agentsctl config init"
      echo "Docs: https://github.com/${REPO}#readme"
      return 0
    fi
  done
  return 1
}

install_from_source() {
  need go
  need git
  echo "→ cloning github.com/${REPO} (${VERSION})"
  if [ "$VERSION" = "latest" ]; then
    git clone --depth 1 "https://github.com/${REPO}.git" "$tmpdir/src"
  else
    git clone --depth 1 --branch "$VERSION" "https://github.com/${REPO}.git" "$tmpdir/src"
  fi
  cd "$tmpdir/src"
  echo "→ building"
  local ver
  ver=$(git describe --tags --always 2>/dev/null || echo dev)
  go build -ldflags "-s -w -X main.version=${ver}" -o agentsd ./cmd/agentsd
  go build -ldflags "-s -w -X main.version=${ver}" -o agentsctl ./cmd/agentsctl
  install_bins "$tmpdir/src"
  echo
  echo "built from source (${ver})"
  echo "Docs: https://github.com/${REPO}#readme"
}

if [ "$SOURCE" = "1" ]; then
  install_from_source
  exit 0
fi

if install_from_release; then
  exit 0
fi

echo "release download failed — falling back to source build" >&2
install_from_source
