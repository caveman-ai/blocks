#!/usr/bin/env sh
# Publish per-platform npm packages from GoReleaser's dist/ output, then the shim package.
# Usage: VERSION=1.2.3 scripts/publish-npm.sh [--dry-run]
set -eu
: "${VERSION:?set VERSION}"
DRY=${1:-}
root=$(cd "$(dirname "$0")/.." && pwd)
# Prereleases (anything with a hyphen, e.g. 0.1.0-rc.1) publish under the "next" dist-tag, never "latest".
case $VERSION in *-*) TAG="--tag next";; *) TAG="";; esac
pub() { if [ "$DRY" = "--dry-run" ]; then echo "would publish $1 $TAG"; else (cd "$1" && npm publish --access public $TAG); fi; }
for target in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64 windows_amd64; do
  os=${target%_*}; arch=${target#*_}
  case $os in windows) nos=win32; exe=.exe;; *) nos=$os; exe=;; esac
  case $arch in amd64) narch=x64;; arm64) narch=arm64;; esac
  src=$(ls -d "$root"/dist/caveman-blocks_${target}*/ 2>/dev/null | head -1) || true
  [ -n "$src" ] || { echo "skip $target: no build"; continue; }
  dir="$root/dist/npm/$nos-$narch"; mkdir -p "$dir"
  cp "$src/caveman-blocks$exe" "$dir/"; chmod 755 "$dir/caveman-blocks$exe"
  sed -e "s/\${OS}/$nos/g" -e "s/\${ARCH}/$narch/g" -e "s/\${VERSION}/$VERSION/g" -e "s/\${EXE}/$exe/g" \
    "$root/npm/platform/package.json.tmpl" > "$dir/package.json"
  pub "$dir"
done
shim="$root/dist/npm/caveman-blocks"; rm -rf "$shim"; cp -R "$root/npm/caveman-blocks" "$shim"
sed -i.bak -e "s/\"0.0.0\"/\"$VERSION\"/g" "$shim/package.json" && rm "$shim/package.json.bak"
pub "$shim"
