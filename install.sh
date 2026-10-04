#!/usr/bin/env sh
# Install caveman-blocks from GitHub releases. Verifies the checksum before installing.
# Usage: curl -fsSL https://caveman.so/blocks/install.sh | sh        (optional: VERSION=v0.1.0 BIN_DIR=~/.local/bin)
set -eu
REPO="caveman-ai/blocks"
BIN_DIR=${BIN_DIR:-"$HOME/.local/bin"}
os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m)
case $arch in x86_64|amd64) arch=amd64;; arm64|aarch64) arch=arm64;; *) echo "unsupported arch $arch" >&2; exit 1;; esac
case $os in darwin|linux) ;; *) echo "unsupported os $os; on Windows use npx caveman-blocks" >&2; exit 1;; esac
if [ -z "${VERSION:-}" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
fi
[ -n "$VERSION" ] || { echo "could not determine latest version" >&2; exit 1; }
base="https://github.com/$REPO/releases/download/$VERSION"
file="caveman-blocks_${VERSION#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$base/$file" -o "$tmp/$file"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
(cd "$tmp" && grep " $file\$" checksums.txt | { if command -v sha256sum >/dev/null; then sha256sum -c -; else shasum -a 256 -c -; fi; })
tar -xzf "$tmp/$file" -C "$tmp"
mkdir -p "$BIN_DIR"; install -m 755 "$tmp/caveman-blocks" "$BIN_DIR/caveman-blocks"
echo "installed $BIN_DIR/caveman-blocks ($VERSION)"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "add $BIN_DIR to your PATH";; esac
echo "next: caveman-blocks hooks install   # once per machine"
echo "      caveman-blocks init            # in a repo"
