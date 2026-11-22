package app

import (
	"archive/zip"
	"crypto/sha256"
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
	if ok {
		for _, platform := range []string{"windows", "linux"} {
			if file, err := openSetupBinary(platform); err == nil {
				file.Close()
				platforms = append(platforms, platform)
			}
		}
	}
	reply(w, http.StatusOK, map[string]any{"origin": origin, "platforms": platforms})
}

func serveSetup(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	origin, ok := setupOrigin(r)
	if !ok {
		fail(w, r, 409, "SETUP_ORIGIN_UNAVAILABLE", "Open Cryol through its HTTPS address or an enabled localhost test address.")
		return
	}
	file, err := openSetupBinary(platform)
	if err != nil {
		fail(w, r, 404, "SETUP_UNAVAILABLE", "This platform's setup bundle is unavailable.")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="cryol-agent-`+platform+`-x64.zip"`)
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
	_, _ = io.Copy(entry, file)
}

func setupLauncher(platform, origin string) (string, string) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(origin)))[:16]
	local := ""
	if strings.HasPrefix(origin, "http://") {
		local = " -local-http"
	}
	if platform == "windows" {
		main := `"%~dp0cryol-agent.exe" setup -server "` + origin + `" -state-dir "%LOCALAPPDATA%\Cryol\Agent\` + key + `" -interval 5m` + local
		script := "@echo off\r\nsetlocal DisableDelayedExpansion\r\nif not defined LOCALAPPDATA (\r\n  echo Local AppData is unavailable.\r\n  pause\r\n  exit /b 1\r\n)\r\necho Cryol Agent setup\r\necho Keep this window open to send inventory. Press Ctrl+C to stop.\r\nif exist \"%~dp0cryol-ca.pem\" (\r\n  " + main + " -ca \"%~dp0cryol-ca.pem\"\r\n) else (\r\n  " + main + "\r\n)\r\nif errorlevel 1 pause\r\n"
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
