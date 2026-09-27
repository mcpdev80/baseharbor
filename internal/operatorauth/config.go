package operatorauth

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	EnvIssuer       = "BASEHARBOR_OPERATOR_OIDC_ISSUER"
	EnvClientID     = "BASEHARBOR_OPERATOR_OIDC_CLIENT_ID"
	EnvScopes       = "BASEHARBOR_OPERATOR_OIDC_SCOPES"
	EnvCallbackPort = "BASEHARBOR_OPERATOR_OIDC_CALLBACK_PORT"
)

var (
	ErrConfigurationRequired = errors.New("operator OIDC configuration is required for managed environments")
	ErrAuthenticationRequired = errors.New("authenticated BaseHarbor operator session is required")
)

type Config struct {
	Provider     string
	Issuer       string
	ClientID     string
	Scopes       []string
	CallbackPort int
}

func (c Config) Validate() error {
	issuer := strings.TrimRight(strings.TrimSpace(c.Issuer), "/")
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("operator OIDC issuer must be an HTTPS URL without query or fragment")
	}
	if strings.TrimSpace(c.ClientID) == "" || strings.ContainsAny(c.ClientID, "\r\n") {
		return errors.New("operator OIDC client id is required")
	}
	if c.CallbackPort < 0 || c.CallbackPort > 65535 {
		return errors.New("operator OIDC callback port is invalid")
	}
	for _, scope := range c.Scopes {
		if strings.TrimSpace(scope) == "" || strings.ContainsAny(scope, " \t\r\n") {
			return fmt.Errorf("operator OIDC scope %q is invalid", scope)
		}
	}
	return nil
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Provider: "external-oidc",
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
	if raw := strings.TrimSpace(os.Getenv(EnvCallbackPort)); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a TCP port", EnvCallbackPort)
		}
		cfg.CallbackPort = port
	}
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return Config{}, ErrConfigurationRequired
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
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

func SessionPath(target, environment string) (string, error) {
	target, err := boundaryToken("target", target)
	if err != nil {
		return "", err
	}
	environment, err = boundaryToken("environment", environment)
	if err != nil {
		return "", err
	}
	base := ""
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" {
		base = filepath.Join(dir, "baseharbor")
	} else {
		dir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(dir, "baseharbor", "sessions")
	}
	return filepath.Join(base, "operator-"+target+"-"+environment+".json"), nil
}

func boundaryToken(label, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("%s is required for operator session", label)
	}
	for i, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.'
		if !valid || (i == 0 && (r == '-' || r == '.')) {
			return "", fmt.Errorf("%s %q is invalid for operator session", label, value)
		}
	}
	if strings.HasSuffix(value, "-") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") {
		return "", fmt.Errorf("%s %q is invalid for operator session", label, value)
	}
	return value, nil
}
