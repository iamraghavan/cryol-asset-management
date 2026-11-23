#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
mkdir -p -- "${1:-"$repo/dist/agent-binaries"}"
output=$(CDPATH='' cd -- "${1:-"$repo/dist/agent-binaries"}" && pwd)
cd "$repo/apps/server"

build() {
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -o "$output/$3" ./cmd/cryol-agent
}

build windows amd64 cryol-agent.exe
build linux amd64 cryol-agent
build linux arm64 cryol-agent-linux-arm64
build darwin amd64 cryol-agent-macos-intel
build darwin arm64 cryol-agent-macos-arm64
