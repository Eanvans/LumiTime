#!/usr/bin/env bash
set -euo pipefail

# Build Lumitime for linux/amd64 (Intel) with no cgo.
# Run this from the repo root on your mac or dev machine.

echo "Running go mod tidy..."
go mod tidy

echo "Building linux/amd64 binary..."
# disable VCS stamping to avoid error when VCS metadata isn't available
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -buildvcs=false -ldflags='-s -w' -o lumitime-linux-amd64 .

echo "Build finished: ./lumitime-linux-amd64"

# Make it executable
chmod +x ./lumitime-linux-amd64

echo "Done. Copy ./lumitime-linux-amd64 and benchlist.json/data.json to your Ubuntu server."