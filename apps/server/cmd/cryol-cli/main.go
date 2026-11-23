package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/auth"
	"github.com/cryol/cryol-asset-management/apps/server/internal/config"
	"github.com/cryol/cryol-asset-management/apps/server/internal/database"
	"golang.org/x/term"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Error("command failed", "error", err)
		os.Exit(1)
	}
	log.Info("database command succeeded", "command", os.Args[1])
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "setup" {
		return runSetup(ctx, args[1:])
	}
	if !(len(args) == 1 && (args[0] == "migrate" || args[0] == "schema-check")) && !(len(args) == 2 && args[0] == "bootstrap-admin") {
		return errors.New("usage: cryol-cli setup [--write-env PATH] [--restore BACKUP] | migrate | schema-check | bootstrap-admin USERNAME")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if args[0] == "migrate" {
		return database.Migrate(ctx, pool)
	}
	if err := database.Check(ctx, pool); err != nil {
		return err
	}
	if args[0] != "bootstrap-admin" {
		return nil
	}
	password, err := readAdminPassword()
	if err != nil {
		return err
	}
	defer clear(password)
	return auth.Bootstrap(ctx, pool, args[1], password)
}

func readAdminPassword() ([]byte, error) {
	in := int(os.Stdin.Fd())
	if !term.IsTerminal(in) {
		return nil, errors.New("administrator bootstrap requires an interactive terminal")
	}
	fmt.Fprint(os.Stderr, "Administrator password: ")
	password, err := term.ReadPassword(in)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	confirmation, err := term.ReadPassword(in)
	fmt.Fprintln(os.Stderr)
	defer clear(confirmation)
	if err != nil {
		clear(password)
		return nil, err
	}
	if !bytes.Equal(password, confirmation) {
		clear(password)
		return nil, errors.New("passwords differ")
	}
	return password, nil
}
