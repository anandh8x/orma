#!/usr/bin/env bash
# Install orma from GitHub releases (linux/mac).
set -euo pipefail

REPO="${ORMA_REPO:-anandh8x/orma}"
VERSION="${ORMA_VERSION:-latest}"
PREFIX="${ORMA_PREFIX:-$HOME/.local}"
BIN_DIR="${PREFIX}/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported os: $OS" >&2; exit 1 ;;
esac

mkdir -p "$BIN_DIR"

if [[ "$VERSION" == "latest" ]]; then
  URL=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n "s/.*\"browser_download_url\": \"\\([^\"]*orma_${OS}_${ARCH}[^\"]*\\)\".*/\\1/p" | head -1)
else
  URL="https://github.com/${REPO}/releases/download/${VERSION}/orma_${OS}_${ARCH}.tar.gz"
fi

if [[ -z "${URL:-}" ]]; then
  echo "no release asset found; build from source:" >&2
  echo "  go install github.com/${REPO}/cmd/orma@latest" >&2
  exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL" -o "$TMP/orma.tgz"
tar -xzf "$TMP/orma.tgz" -C "$TMP"
install -m 755 "$TMP/orma" "$BIN_DIR/orma"
echo "installed $BIN_DIR/orma"
echo "run: orma init"
echo "then: eval \"\$(orma hook zsh)\"  # or bash"
