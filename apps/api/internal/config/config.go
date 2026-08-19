package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr       string
	DevAuth        bool
	DatabaseURL    string
	CORSOrigin     string
	ReservationTTL time.Duration
}

func FromEnv() (Config, error) {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8098"
	}
	devAuth := strings.EqualFold(os.Getenv("COMMERCE_DEV_AUTH"), "true") || os.Getenv("COMMERCE_DEV_AUTH") == "1"
	ttl := 15 * time.Minute
	cfg := Config{
		HTTPAddr:       ":" + port,
		DevAuth:        devAuth,
		DatabaseURL:    strings.TrimSpace(os.Getenv("COMMERCE_DATABASE_URL")),
		CORSOrigin:     strings.TrimSpace(os.Getenv("COMMERCE_CORS_ORIGIN")),
		ReservationTTL: ttl,
	}
	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "http://localhost:3008"
	}
	if !cfg.DevAuth {
		return cfg, fmt.Errorf("COMMERCE_DEV_AUTH=true is required in this slice (P01 OIDC is not wired yet)")
	}
	return cfg, nil
}
