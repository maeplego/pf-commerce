package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	DevAuth      bool
	DatabaseURL  string
	CORSOrigin   string
	CatalogURL   string
	InventoryURL string
	OrderURL     string
	NotifyURL    string
}

func FromEnv() (Config, error) {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8099"
	}
	devAuth := strings.EqualFold(os.Getenv("COMMERCE_DEV_AUTH"), "true") || os.Getenv("COMMERCE_DEV_AUTH") == "1"
	cfg := Config{
		HTTPAddr:     ":" + port,
		DevAuth:      devAuth,
		DatabaseURL:  strings.TrimSpace(os.Getenv("COMMERCE_DATABASE_URL")),
		CORSOrigin:   strings.TrimSpace(os.Getenv("COMMERCE_CORS_ORIGIN")),
		CatalogURL:   strings.TrimSpace(os.Getenv("COMMERCE_CATALOG_URL")),
		InventoryURL: strings.TrimSpace(os.Getenv("COMMERCE_INVENTORY_URL")),
		OrderURL:     strings.TrimSpace(os.Getenv("COMMERCE_ORDER_URL")),
		NotifyURL:    strings.TrimSpace(os.Getenv("COMMERCE_NOTIFY_URL")),
	}
	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "http://localhost:3009,http://localhost:3010,http://localhost:8110"
	}
	if !cfg.DevAuth {
		return cfg, fmt.Errorf("COMMERCE_DEV_AUTH=true is required in this slice (P01 OIDC is not wired yet)")
	}
	if cfg.CatalogURL == "" || cfg.InventoryURL == "" || cfg.OrderURL == "" {
		return cfg, fmt.Errorf("COMMERCE_CATALOG_URL, COMMERCE_INVENTORY_URL, COMMERCE_ORDER_URL are required")
	}
	return cfg, nil
}
