#!/bin/sh
set -eu
umask 077
cd /var/lib/cryol/tls

if [ -s server.pem ] && [ -s server.key ]; then
    openssl x509 -in server.pem -noout -checkend 0
    exit
fi
if [ -e server.pem ] || [ -e server.key ]; then
    echo "Incomplete local TLS setup. Restore the matching certificate and key before retrying." >&2
    exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 365 \
    -subj '/CN=Cryol localhost' \
    -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' \
    -addext 'basicConstraints=critical,CA:FALSE' \
    -keyout "$tmp/server.key" -out "$tmp/server.pem"
mv "$tmp/server.key" "$tmp/server.pem" .
