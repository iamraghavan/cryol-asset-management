package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleDocumentsAndMissingAssetsAreNotCached(t *testing.T) {
	web := t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<!doctype html><title>Cryol</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := Handler(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, "test", web)
	for path, status := range map[string]int{"/console/agents": http.StatusOK, "/assets/old-console.js": http.StatusNotFound} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s returned %d with cache control %q", path, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}

func TestSystemRoutes(t *testing.T) {
	unavailable := false
	handler := Handler(slog.New(slog.NewTextHandler(io.Discard, nil)), func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness check needs a deadline")
		}
		if unavailable {
			return errors.New("private database diagnostic")
		}
		return nil
	}, "test-version", "")

	for _, tt := range []struct {
		path   string
		fail   bool
		status int
		body   string
	}{
		{"/health", true, http.StatusOK, "alive"},
		{"/version", true, http.StatusOK, "test-version"},
		{"/readiness", false, http.StatusOK, "ready"},
		{"/readiness", true, http.StatusServiceUnavailable, "NOT_READY"},
	} {
		t.Run(tt.path+tt.body, func(t *testing.T) {
			unavailable = tt.fail
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system"+tt.path, nil))
			body := response.Body.String()
			if response.Code != tt.status || !strings.Contains(body, tt.body) || strings.Contains(body, "private database diagnostic") {
				t.Fatalf("unexpected response %d %s", response.Code, body)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request ID")
			}
		})
	}
}
