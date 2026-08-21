package envprofile

import (
	"fmt"
	"strings"
)

const (
	Development = "development"
	Staging     = "staging"
	Production  = "production"
)

// Normalize maps empty/local aliases to development.
func Normalize(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "dev", "development", "local", "demo":
		return Development
	case "staging", "stage":
		return Staging
	case "production", "prod":
		return Production
	default:
		return strings.ToLower(strings.TrimSpace(v))
	}
}

// ValidateCommercial rejects DEV_AUTH and missing OIDC on staging/production.
// envVar / devAuthVar are names used in error messages (e.g. COMMERCE_ENV).
func ValidateCommercial(env string, devAuth bool, oidcIssuer, envVar, devAuthVar string) error {
	switch env {
	case Development, Staging, Production:
	default:
		return fmt.Errorf("unsupported %s %q (use development, staging, or production)", envVar, env)
	}
	if (env == Staging || env == Production) && devAuth {
		return fmt.Errorf("%s must be false when %s=%s", devAuthVar, envVar, env)
	}
	if (env == Staging || env == Production) && strings.TrimSpace(oidcIssuer) == "" {
		return fmt.Errorf("OIDC_ISSUER is required when %s=%s", envVar, env)
	}
	return nil
}
