package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mcpdev80/baseharbor/internal/identity"
)

// OIDCVerifier verifies ID tokens using OIDC discovery and the provider JWKS.
// One verifier is created per accepted audience so the upstream library keeps
// ownership of issuer, signature, expiry and authorized-party validation.
type OIDCVerifier struct {
	issuer    string
	verifiers []*oidc.IDTokenVerifier
}

// NewOIDCVerifier discovers the OIDC provider and prepares verifiers for every
// configured audience. Discovery must succeed before the verifier is usable.
func NewOIDCVerifier(ctx context.Context, cfg Config) (*OIDCVerifier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	provider, err := oidc.NewProvider(ctx, strings.TrimSpace(cfg.Issuer))
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return newOIDCVerifier(strings.TrimSpace(cfg.Issuer), cfg.Audiences, func(audience string) *oidc.IDTokenVerifier {
		return provider.Verifier(&oidc.Config{ClientID: audience})
	})
}

func newOIDCVerifier(issuer string, audiences []string, makeVerifier func(string) *oidc.IDTokenVerifier) (*OIDCVerifier, error) {
	if strings.TrimSpace(issuer) == "" || len(audiences) == 0 || makeVerifier == nil {
		return nil, errors.New("OIDC verifier configuration is incomplete")
	}

	verifiers := make([]*oidc.IDTokenVerifier, 0, len(audiences))
	seen := make(map[string]struct{}, len(audiences))
	for _, audience := range audiences {
		audience = strings.TrimSpace(audience)
		if audience == "" {
			return nil, ErrMissingAudience
		}
		if _, ok := seen[audience]; ok {
			continue
		}
		seen[audience] = struct{}{}
		verifiers = append(verifiers, makeVerifier(audience))
	}
	if len(verifiers) == 0 {
		return nil, ErrMissingAudience
	}

	return &OIDCVerifier{issuer: issuer, verifiers: verifiers}, nil
}

// Verify validates the token against one of the configured audiences and
// returns only provider-neutral identity claims. Validation failures are
// intentionally collapsed so raw token material never reaches callers.
func (v *OIDCVerifier) Verify(ctx context.Context, rawToken string) (*identity.Principal, error) {
	if v == nil || len(v.verifiers) == 0 || strings.TrimSpace(rawToken) == "" {
		return nil, ErrInvalidToken
	}

	for _, verifier := range v.verifiers {
		token, err := verifier.Verify(ctx, rawToken)
		if err != nil {
			continue
		}
		if token.Issuer != v.issuer || token.Subject == "" {
			return nil, ErrInvalidToken
		}
		return &identity.Principal{
			Issuer:   token.Issuer,
			Subject:  token.Subject,
			Audience: append([]string(nil), token.Audience...),
		}, nil
	}
	return nil, ErrInvalidToken
}
