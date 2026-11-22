//go:build !windows

package main

import (
	"context"
	"log/slog"
	"os"
)

func platformCommand(_ context.Context, _ *slog.Logger, _ []string) (bool, error) {
	return false, nil
}

func lockAgent(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	return func() { _ = os.Remove(path) }, nil
}
