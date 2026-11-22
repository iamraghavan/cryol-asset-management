package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
	WebDir      string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(os.Getenv("CRYOL_DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(os.Getenv("CRYOL_HTTP_ADDR")),
		WebDir:      strings.TrimSpace(os.Getenv("CRYOL_WEB_DIR")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("CRYOL_DATABASE_URL is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:8080"
	}
	return cfg, nil
}
