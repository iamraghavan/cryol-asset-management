#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
out="${CRYOL_WINDOWS_BUILD_DIR:-$root/dist/windows-agent}"
mkdir -p "$out"
out=$(CDPATH= cd -- "$out" && pwd)
compiler=${MAKENSIS:-makensis}

(
  cd "$root/apps/server"
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=0.3.3" -o "$out/cryol-agent.exe" ./cmd/cryol-agent
)

# For release signing, sign the agent first and invoke makensis separately.
cd "$root/installer/windows/agent"
set -- -V3 "-DAGENT_EXE=$out/cryol-agent.exe" "-DOUTPUT_EXE=$out/Cryol-Agent-Setup-x64.exe"
if [ -n "${CRYOL_WINDOWS_DEFAULT_SERVER:-}" ]; then
  case "$CRYOL_WINDOWS_DEFAULT_SERVER" in
    *[!A-Za-z0-9.:/\[\]-]*) echo "Default server contains unsupported characters" >&2; exit 1 ;;
    https://*) set -- "$@" "-DDEFAULT_SERVER=$CRYOL_WINDOWS_DEFAULT_SERVER" ;;
    *) echo "Default server must use HTTPS" >&2; exit 1 ;;
  esac
fi
if [ -n "${CRYOL_WINDOWS_PUBLIC_CA:-}" ]; then
  python3 - "$CRYOL_WINDOWS_PUBLIC_CA" "$out/cryol-ca.pem" <<'PYCA'
import pathlib, re, subprocess, sys

source, target = map(pathlib.Path, sys.argv[1:])
data = source.read_bytes()
if not data or len(data) > 128 * 1024:
    raise SystemExit("Invalid public CA file size")
certificates = re.findall(rb"-----BEGIN CERTIFICATE-----\s+[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----", data)
if not certificates or re.sub(rb"-----BEGIN CERTIFICATE-----\s+[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----", b"", data).strip():
    raise SystemExit("Use only public PEM certificate blocks")
for certificate in certificates:
    check = subprocess.run(["openssl", "x509", "-noout", "-text"], input=certificate,
                           capture_output=True, check=True)
    if b"CA:TRUE" not in check.stdout:
        raise SystemExit("The setup trust file must contain CA certificates")
target.write_bytes(b"\n".join(cert.replace(b"\r", b"") for cert in certificates) + b"\n")
PYCA
  set -- "$@" "-DPUBLIC_CA_PEM=$out/cryol-ca.pem"
fi
"$compiler" "$@" setup.nsi
(
  cd "$out"
  shasum -a 256 Cryol-Agent-Setup-x64.exe cryol-agent.exe > SHA256SUMS.txt
)
