package app

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSetupBundleOriginAndContents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRYOL_AGENT_DOWNLOAD_DIR", dir)
	t.Setenv("CRYOL_AGENT_LOCAL_HTTP", "1")
	if err := os.WriteFile(filepath.Join(dir, "cryol-agent.exe"), []byte("test binary"), 0600); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/setup/{platform}", serveSetup)
	for _, example := range []struct {
		name, origin, peer string
		status             int
		local              bool
	}{
		{"local", "http://localhost:8080", "127.0.0.1:23456", 200, true},
		{"https", "https://192.0.2.3:8443", "192.0.2.2:23456", 200, false},
		{"remote HTTP", "http://192.0.2.3:8080", "192.0.2.2:23456", 409, false},
		{"spoofed local", "http://localhost:8080", "192.0.2.2:23456", 409, false},
	} {
		t.Run(example.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", example.origin+"/setup/windows", nil)
			r.RemoteAddr = example.peer
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != example.status {
				t.Fatalf("got %d want %d", w.Code, example.status)
			}
			if w.Code != 200 {
				return
			}
			bundle, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.File) != 2 || bundle.File[0].Name != "Start Cryol Agent.cmd" || bundle.File[1].Name != "cryol-agent.exe" {
				t.Fatal("unexpected setup bundle entries")
			}
			entry, err := bundle.File[0].Open()
			if err != nil {
				t.Fatal(err)
			}
			defer entry.Close()
			source, err := io.ReadAll(entry)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), example.origin) || strings.Contains(string(source), "-local-http") != example.local {
				t.Fatalf("launcher origin or local test mode is incorrect")
			}
		})
	}
	t.Setenv("CRYOL_AGENT_LOCAL_HTTP", "0")
	r := httptest.NewRequest("GET", "http://localhost:8080/setup/windows", nil)
	r.RemoteAddr = "127.0.0.1:23456"
	if _, ok := setupOrigin(r); ok {
		t.Fatal("loopback HTTP allowed without opt-in")
	}
	r = httptest.NewRequest("GET", "https://cryol.test/setup/windows", nil)
	r.Host = `cryol.test" & unwanted`
	if _, ok := setupOrigin(r); ok {
		t.Fatal("unsafe host accepted")
	}
}
