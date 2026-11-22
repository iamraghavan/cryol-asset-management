package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
