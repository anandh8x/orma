#!/usr/bin/env bash
# Install a runnable orma into ~/.local/bin (or ORMA_PREFIX).
set -euo pipefail

PREFIX="${ORMA_PREFIX:-$HOME/.local}"
BINDIR="${PREFIX}/bin"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$BINDIR"

if command -v go >/dev/null 2>&1; then
  echo "building with go..."
  (
    cd "$REPO_ROOT"
    make install PREFIX="$PREFIX" VERSION="${ORMA_VERSION:-dev}"
  )
else
  echo "go not found; cannot build from source" >&2
  exit 1
fi

case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *)
    echo "note: add to PATH: export PATH=\"$BINDIR:\$PATH\""
    ;;
esac

echo
echo "next:"
echo "  orma init"
echo "  eval \"\$(orma hook zsh)\"   # put in ~/.zshrc"
echo "  # or bash: eval \"\$(orma hook bash)\""
