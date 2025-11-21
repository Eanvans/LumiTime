#!/usr/bin/env bash
set -euo pipefail

# prepare_release.sh
# Move the built binary and config files into this release/ folder
# Run this from the project root after you've built the linux binary.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RELEASE_DIR="$ROOT_DIR/release"

echo "Preparing release in: $RELEASE_DIR"

mkdir -p "$RELEASE_DIR"

# binary
if [ -f "$ROOT_DIR/lumitime-linux-amd64" ]; then
  cp -v "$ROOT_DIR/lumitime-linux-amd64" "$RELEASE_DIR/"
else
  echo "warning: $ROOT_DIR/lumitime-linux-amd64 not found; build it first with ./scripts/build_linux_amd64.sh"
fi

# optional files
for f in benchlist.json data.json; do
  if [ -f "$ROOT_DIR/$f" ]; then
    cp -v "$ROOT_DIR/$f" "$RELEASE_DIR/"
  fi
done

# copy systemd template if present
if [ -f "$ROOT_DIR/deploy/lumitime.service" ]; then
  cp -v "$ROOT_DIR/deploy/lumitime.service" "$RELEASE_DIR/"
fi

# create tarball
pushd "$RELEASE_DIR" >/dev/null
TARBALL="lumitime-release-$(date +%Y%m%d%H%M%S).tar.gz"
rm -f lumitime-release.tar.gz || true
tar -czf "$TARBALL" .
ln -sf "$TARBALL" lumitime-release.tar.gz
ls -la "$TARBALL" lumitime-release.tar.gz
popd >/dev/null

echo "Release prepared: $RELEASE_DIR/lumitime-release.tar.gz"
