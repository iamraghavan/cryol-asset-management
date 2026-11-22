package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Handler(log *slog.Logger, check func(context.Context) error, version, webDir string, mounts ...func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Compress(5, "text/html", "text/css", "application/json", "text/javascript", "application/javascript"))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; form-action 'self'")
			w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
			next.ServeHTTP(w, r)
		})
	})
	r.Route("/api/v1/system", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			reply(w, http.StatusOK, map[string]string{"status": "alive"})
		})
		r.Get("/version", func(w http.ResponseWriter, _ *http.Request) {
			reply(w, http.StatusOK, map[string]string{"version": version})
		})
		r.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := check(ctx); err != nil {
				requestID := middleware.GetReqID(ctx)
				log.WarnContext(ctx, "readiness check failed", "request_id", requestID)
				reply(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{
					"code": "NOT_READY", "message": "Database or schema unavailable.", "requestId": requestID,
				}})
				return
			}
			reply(w, http.StatusOK, map[string]string{"status": "ready"})
		})
	})
	for _, mount := range mounts {
		mount(r)
	}
	if webDir != "" {
		serveIndex := func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
		}
		r.Get("/", serveIndex)
		r.Get("/system", serveIndex)
		r.Get("/console", serveIndex)
		r.Get("/console/dashboard", serveIndex)
		r.Get("/console/assets", serveIndex)
		r.Get("/console/agents", serveIndex)
		r.Get("/console/settings", serveIndex)
		r.Get("/console/settings/organization", serveIndex)
		r.Get("/assets/*", func(w http.ResponseWriter, r *http.Request) {
			name := chi.URLParam(r, "*")
			if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:") {
				http.NotFound(w, r)
				return
			}
			path := filepath.Join(webDir, "assets", name)
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, path)
		})
	}
	return r
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
