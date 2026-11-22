package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"github.com/go-chi/chi/v5"
)

var setupHost = regexp.MustCompile(`\A[A-Za-z0-9.\-:\[\]]+\z`)

func agentLoopbackHTTP(r *http.Request) bool {
	if os.Getenv("CRYOL_AGENT_LOCAL_HTTP") != "1" {
		return false
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(peer) == nil || !inventory.IsLoopbackHost(peer) {
		return false
	}
	if !setupHost.MatchString(r.Host) {
		return false
	}
	host, err := url.Parse("http://" + r.Host)
	return err == nil && inventory.IsLoopbackHost(host.Hostname())
}

func setupOrigin(r *http.Request) (string, bool) {
	if configured := os.Getenv("CRYOL_AGENT_SETUP_ORIGIN"); configured != "" {
		origin, err := url.Parse(configured)
		if err != nil || origin.Scheme != "https" || !setupHost.MatchString(origin.Host) || origin.Hostname() == "" ||
			origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery {
			return "", false
		}
		return "https://" + origin.Host, true
	}
	if !setupHost.MatchString(r.Host) {
		return "", false
	}
	scheme := "https"
	if r.TLS == nil {
		if !agentLoopbackHTTP(r) {
			return "", false
		}
		scheme = "http"
	}
	origin, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || origin.Hostname() == "" {
		return "", false
	}
	return origin.String(), true
}

func setupBinary(platform string) string {
	switch platform {
	case "windows":
		return "cryol-agent.exe"
	case "linux":
		return "cryol-agent"
	default:
		return ""
	}
}

func openSetupBinary(platform string) (*os.File, error) {
	dir, name := os.Getenv("CRYOL_AGENT_DOWNLOAD_DIR"), setupBinary(platform)
	if dir == "" || name == "" {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		file.Close()
		return nil, os.ErrInvalid
	}
	return file, nil
}

func setupOptions(w http.ResponseWriter, r *http.Request) {
	origin, ok := setupOrigin(r)
	platforms := []string{}
	installerAvailable, caAvailable := false, false
	if ca, err := setupCA(); ok && err == nil {
		caAvailable = len(ca) > 0
		for _, platform := range []string{"windows", "linux"} {
			if file, err := openSetupBinary(platform); err == nil {
				file.Close()
				platforms = append(platforms, platform)
			}
		}
		if strings.HasPrefix(origin, "https://") {
			if file, err := openWindowsInstaller(); err == nil {
				file.Close()
				installerAvailable = true
			}
		}
	}
	reply(w, http.StatusOK, map[string]any{
		"origin": origin, "platforms": platforms, "windowsInstaller": installerAvailable, "publicCA": caAvailable,
	})
}

func openWindowsInstaller() (*os.File, error) {
	path := os.Getenv("CRYOL_WINDOWS_AGENT_INSTALLER")
	if path == "" {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 2 || info.Size() > 128<<20 {
		file.Close()
		return nil, os.ErrInvalid
	}
	var magic [2]byte
	if _, err = io.ReadFull(file, magic[:]); err != nil || string(magic[:]) != "MZ" {
		file.Close()
		return nil, os.ErrInvalid
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func serveWindowsInstaller(w http.ResponseWriter, r *http.Request) {
	origin, ok := setupOrigin(r)
	if !ok || !strings.HasPrefix(origin, "https://") {
		fail(w, r, 409, "SETUP_ORIGIN_UNAVAILABLE", "Configure the endpoint-facing HTTPS server before downloading the installer.")
		return
	}
	if _, err := setupCA(); err != nil {
		fail(w, r, 503, "SETUP_CA_UNAVAILABLE", "The setup CA must contain only public CA certificates.")
		return
	}
	file, err := openWindowsInstaller()
	if err != nil {
		fail(w, r, 404, "SETUP_UNAVAILABLE", "The Windows installer is unavailable.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		fail(w, r, 503, "SETUP_UNAVAILABLE", "The Windows installer cannot be read.")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="Cryol-Agent-Setup-x64.exe"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "Cryol-Agent-Setup-x64.exe", info.ModTime(), file)
}

func serveSetupCA(w http.ResponseWriter, r *http.Request) {
	ca, err := setupCA()
	if err != nil || len(ca) == 0 {
		fail(w, r, 404, "SETUP_CA_UNAVAILABLE", "The public setup CA is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="cryol-ca.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(ca)
}

func serveSetup(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	origin, ok := setupOrigin(r)
	if !ok {
		fail(w, r, 409, "SETUP_ORIGIN_UNAVAILABLE", "Open Cryol through its HTTPS address or an enabled localhost test address.")
		return
	}
	ca, err := setupCA()
	if err != nil {
		fail(w, r, 503, "SETUP_CA_UNAVAILABLE", "The setup CA must contain only public CA certificates.")
		return
	}
	file, err := openSetupBinary(platform)
	if err != nil {
		fail(w, r, 404, "SETUP_UNAVAILABLE", "This platform's setup bundle is unavailable.")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/zip")
	filename := "cryol-agent-linux-x64.zip"
	if platform == "windows" {
		filename = "cryol-agent-windows-service-x64.zip"
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	archive := zip.NewWriter(w)
	defer archive.Close()
	name, script := setupLauncher(platform, origin)
	entry, err := archive.Create(name)
	if err != nil {
		return
	}
	if _, err = io.WriteString(entry, script); err != nil {
		return
	}
	entry, err = archive.Create(setupBinary(platform))
	if err != nil {
		return
	}
	if _, err = io.Copy(entry, file); err != nil {
		return
	}
	if len(ca) > 0 {
		entry, err = archive.Create("cryol-ca.pem")
		if err == nil {
			_, _ = entry.Write(ca)
		}
	}
}

func setupCA() ([]byte, error) {
	path := os.Getenv("CRYOL_AGENT_SETUP_CA")
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const max = 128 << 10
	raw, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || len(raw) > max {
		return nil, errors.New("invalid setup CA file")
	}
	var output bytes.Buffer
	for data := bytes.TrimSpace(raw); len(data) > 0; data = bytes.TrimSpace(data) {
		if !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("invalid setup CA block")
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid setup CA block")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !certificate.IsCA {
			return nil, errors.New("invalid public CA certificate")
		}
		// Encode only the parsed public certificate, never the raw input file.
		if err := pem.Encode(&output, &pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}); err != nil {
			return nil, err
		}
		data = rest
	}
	if output.Len() == 0 {
		return nil, errors.New("empty setup CA bundle")
	}
	return output.Bytes(), nil
}

func setupLauncher(platform, origin string) (string, string) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(origin)))[:16]
	local := ""
	if strings.HasPrefix(origin, "http://") {
		local = " -local-http"
	}
	if platform == "windows" {
		main := `"%~dp0cryol-agent.exe" install-service -server "` + origin + `" -interval 5m -show-token` + local
		script := "@echo off\r\nsetlocal DisableDelayedExpansion\r\necho Cryol Endpoint Inventory setup\r\necho Approve the Windows administrator prompt, then paste the enrollment token.\r\nif exist \"%~dp0cryol-ca.pem\" (\r\n  " + main + " -ca \"%~dp0cryol-ca.pem\" -pause\r\n) else (\r\n  " + main + " -pause\r\n)\r\nif errorlevel 1 pause\r\n"
		return "Start Cryol Agent.cmd", script
	}
	return "start-cryol-agent.sh", `#!/bin/sh
set -eu
cd -- "$(dirname -- "$0")"
umask 077
chmod u+x ./cryol-agent
echo 'Keep this terminal open to send inventory. Press Ctrl+C to stop.'
if [ -f ./cryol-ca.pem ]; then
  set -- -ca ./cryol-ca.pem
else
  set --
fi
exec ./cryol-agent setup -server '` + origin + `' -state-dir "${XDG_STATE_HOME:-$HOME/.local/state}/cryol-agent/` + key + `" -interval 5m` + local + ` "$@"
`
}
