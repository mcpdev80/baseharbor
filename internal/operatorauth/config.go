package operatorauth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvIssuer   = "BASEHARBOR_OPERATOR_OIDC_ISSUER"
	EnvClientID = "BASEHARBOR_OPERATOR_OIDC_CLIENT_ID"
	EnvScopes   = "BASEHARBOR_OPERATOR_OIDC_SCOPES"
)

var (
	ErrConfigurationRequired = errors.New("operator OIDC configuration is required for managed environments")
	ErrAuthenticationRequired = errors.New("authenticated BaseHarbor operator session is required")
)

type Config struct {
	Issuer   string
	ClientID string
	Scopes   []string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Issuer:   strings.TrimSpace(os.Getenv(EnvIssuer)),
		ClientID: strings.TrimSpace(os.Getenv(EnvClientID)),
	}
	for _, raw := range strings.Split(os.Getenv(EnvScopes), ",") {
		if value := strings.TrimSpace(raw); value != "" {
			cfg.Scopes = append(cfg.Scopes, value)
		}
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return Config{}, ErrConfigurationRequired
	}
	return cfg, nil
}

func ManagedEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return false
	default:
		return true
	}
}

func SessionPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" {
		return filepath.Join(dir, "baseharbor", "operator-session.json"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "baseharbor", "operator-session.json"), nil
}
