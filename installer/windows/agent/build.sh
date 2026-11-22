#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
out="${CRYOL_WINDOWS_BUILD_DIR:-$root/dist/windows-agent}"
mkdir -p "$out"
out=$(CDPATH= cd -- "$out" && pwd)
compiler=${MAKENSIS:-makensis}

(
  cd "$root/apps/server"
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$out/cryol-agent.exe" ./cmd/cryol-agent
)

# For release signing, sign the agent first and invoke makensis separately.
cd "$root/installer/windows/agent"
set -- -V3 "-DAGENT_EXE=$out/cryol-agent.exe" "-DOUTPUT_EXE=$out/Cryol-Agent-Setup-x64.exe"
if [ -n "${CRYOL_WINDOWS_DEFAULT_SERVER:-}" ]; then
  case "$CRYOL_WINDOWS_DEFAULT_SERVER" in
    *[!A-Za-z0-9.:/-]*) echo "Default server contains unsupported characters" >&2; exit 1 ;;
    https://*) set -- "$@" "-DDEFAULT_SERVER=$CRYOL_WINDOWS_DEFAULT_SERVER" ;;
    *) echo "Default server must use HTTPS" >&2; exit 1 ;;
  esac
fi
"$compiler" "$@" setup.nsi
(
  cd "$out"
  shasum -a 256 Cryol-Agent-Setup-x64.exe cryol-agent.exe > SHA256SUMS.txt
)
