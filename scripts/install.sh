#!/usr/bin/env bash
# Install orma into ~/.local/bin (or ORMA_PREFIX).
#
#   curl -fsSL https://raw.githubusercontent.com/anandh8x/orma/main/scripts/install.sh | bash
#
# Optional env:
#   ORMA_VERSION=v0.1.0   # default: latest release
#   ORMA_PREFIX=~/.local  # install prefix (bin goes in $PREFIX/bin)
#   ORMA_REPO=anandh8x/orma
set -euo pipefail

REPO="${ORMA_REPO:-anandh8x/orma}"
PREFIX="${ORMA_PREFIX:-$HOME/.local}"
BINDIR="${PREFIX}/bin"
VERSION="${ORMA_VERSION:-latest}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd tar
need_cmd install
need_cmd uname

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *)
    echo "unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac
case "$OS" in
  linux | darwin) ;;
  *)
    echo "unsupported OS: $OS (linux/darwin only)" >&2
    exit 1
    ;;
esac

resolve_version() {
  if [[ "$VERSION" != "latest" ]]; then
    echo "$VERSION"
    return
  fi
  # GitHub API: latest tag
  local tag
  tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -1)
  if [[ -z "$tag" ]]; then
    echo "could not resolve latest release for ${REPO}" >&2
    exit 1
  fi
  echo "$tag"
}

download_release() {
  local ver="$1"
  local asset="orma_${OS}_${ARCH}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/${ver}/${asset}"
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN

  echo "downloading ${url}"
  if ! curl -fsSL "$url" -o "${tmp}/${asset}"; then
    return 1
  fi
  tar -xzf "${tmp}/${asset}" -C "$tmp"
  if [[ ! -f "${tmp}/orma" ]]; then
    echo "archive missing orma binary" >&2
    return 1
  fi
  mkdir -p "$BINDIR"
  install -m 755 "${tmp}/orma" "${BINDIR}/orma"
  return 0
}

install_from_go() {
  need_cmd go
  local ver="$1"
  local mod="github.com/${REPO}/cmd/orma@${ver}"
  if [[ "$ver" == "latest" ]]; then
    mod="github.com/${REPO}/cmd/orma@latest"
  fi
  echo "building with go install ${mod}"
  GOBIN="$BINDIR" go install "${mod}"
}

main() {
  local ver
  ver=$(resolve_version)
  echo "installing orma ${ver} -> ${BINDIR}/orma"

  if ! download_release "$ver"; then
    echo "release binary not available; falling back to go install"
    install_from_go "$ver"
  fi

  case ":$PATH:" in
    *":$BINDIR:"*) ;;
    *)
      echo "note: add to PATH: export PATH=\"${BINDIR}:\$PATH\""
      ;;
  esac

  echo
  "${BINDIR}/orma" version 2>/dev/null || true
  echo "installed ${BINDIR}/orma"
  echo "next:"
  echo "  orma init"
  echo "  eval \"\$(orma hook zsh)\"   # or: eval \"\$(orma hook bash)\""
}

main "$@"
