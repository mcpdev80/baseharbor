package auth

import (
	"errors"
	"net/url"
	"strings"
)

var (
	ErrInvalidIssuer   = errors.New("invalid oidc issuer")
	ErrMissingAudience = errors.New("missing oidc audience")
)

// Config describes provider-neutral OIDC verification requirements.
type Config struct {
	Issuer    string
	Audiences []string
}

// Validate rejects incomplete or insecure OIDC configuration.
func (c Config) Validate() error {
	issuer := strings.TrimSpace(c.Issuer)
	if issuer == "" {
		return ErrInvalidIssuer
	}

	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalidIssuer
	}

	if len(c.Audiences) == 0 {
		return ErrMissingAudience
	}
	for _, audience := range c.Audiences {
		if strings.TrimSpace(audience) == "" {
			return ErrMissingAudience
		}
	}

	return nil
}
