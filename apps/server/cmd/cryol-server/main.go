package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/app"
	"github.com/cryol/cryol-asset-management/apps/server/internal/config"
	"github.com/cryol/cryol-asset-management/apps/server/internal/database"
	"github.com/go-chi/chi/v5"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cert, key := os.Getenv("CRYOL_TLS_CERT"), os.Getenv("CRYOL_TLS_KEY")
	host, _, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return errors.New("CRYOL_HTTP_ADDR must contain a host and port")
	}
	if (cert == "") != (key == "") {
		return errors.New("configure both CRYOL_TLS_CERT and CRYOL_TLS_KEY")
	}
	if cert == "" && !net.ParseIP(host).IsLoopback() {
		return errors.New("TLS is required when listening outside a loopback IP")
	}
	if cfg.WebDir != "" {
		info, err := os.Stat(filepath.Join(cfg.WebDir, "index.html"))
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("CRYOL_WEB_DIR must contain a built index.html")
		}
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	check := func(ctx context.Context) error { return database.Check(ctx, pool) }
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = check(startup)
	cancel()
	if err != nil {
		return fmt.Errorf("database startup check failed: %w", err)
	}
	handler := app.Handler(log, check, version, cfg.WebDir, func(r chi.Router) {
		app.MountIdentity(r, pool)
		app.MountAssets(r, pool)
		app.MountAgents(r, pool)
	})
	if cert == "" {
		local := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			if host != "localhost" && !net.ParseIP(strings.Trim(host, "[]")).IsLoopback() {
				http.Error(w, "Forbidden host", http.StatusForbidden)
				return
			}
			local.ServeHTTP(w, r)
		})
	}
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	stopped := make(chan error, 1)
	go func() {
		log.Info("starting HTTP listener", "address", cfg.HTTPAddr, "version", version)
		if cert != "" {
			stopped <- server.ListenAndServeTLS(cert, key)
			return
		}
		stopped <- server.ListenAndServe()
	}()
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		err := <-stopped
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
