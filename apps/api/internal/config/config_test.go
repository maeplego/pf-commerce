package config

import "testing"

func TestFromEnvStagingRejectsDevAuth(t *testing.T) {
	t.Setenv("COMMERCE_ENV", "staging")
	t.Setenv("COMMERCE_DEV_AUTH", "true")
	t.Setenv("OIDC_ISSUER", "http://idp.example")
	t.Setenv("COMMERCE_CATALOG_URL", "http://catalog")
	t.Setenv("COMMERCE_INVENTORY_URL", "http://inventory")
	t.Setenv("COMMERCE_ORDER_URL", "http://order")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error when staging enables COMMERCE_DEV_AUTH")
	}
}

func TestFromEnvStagingRequiresOIDC(t *testing.T) {
	t.Setenv("COMMERCE_ENV", "staging")
	t.Setenv("COMMERCE_DEV_AUTH", "false")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("COMMERCE_CATALOG_URL", "http://catalog")
	t.Setenv("COMMERCE_INVENTORY_URL", "http://inventory")
	t.Setenv("COMMERCE_ORDER_URL", "http://order")
	if _, err := FromEnv(); err == nil {
		t.Fatal("staging must require OIDC_ISSUER")
	}
}

func TestFromEnvStagingOK(t *testing.T) {
	t.Setenv("COMMERCE_ENV", "staging")
	t.Setenv("COMMERCE_DEV_AUTH", "false")
	t.Setenv("OIDC_ISSUER", "http://idp.example")
	t.Setenv("COMMERCE_CATALOG_URL", "http://catalog")
	t.Setenv("COMMERCE_INVENTORY_URL", "http://inventory")
	t.Setenv("COMMERCE_ORDER_URL", "http://order")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "staging" || cfg.DevAuth {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}
