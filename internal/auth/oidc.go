package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

// extractClaimStringSet normalizes one or more claim values that may be a single
// string, a slice of strings, or absent into a sorted, de-duplicated, trimmed
// string slice. It never leaks structured token material beyond the plain claim
// values and is safe for a nil receiver.
func extractClaimStringSet(values ...any) []string {
	seen := make(map[string]struct{})
	for _, v := range values {
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				seen[s] = struct{}{}
			}
		case []string:
			for _, s := range t {
				if s = strings.TrimSpace(s); s != "" {
					seen[s] = struct{}{}
				}
			}
		case []any:
			for _, item := range t {
				if s, ok := item.(string); ok {
					if s = strings.TrimSpace(s); s != "" {
						seen[s] = struct{}{}
					}
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	roles := make([]string, 0, len(seen))
	for role := range seen {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
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
		var claims struct {
			ACR      string   `json:"acr"`
			AMR      []string `json:"amr"`
			Groups   any      `json:"group"`
			Groups2  any      `json:"groups"`
			Roles    any      `json:"roles"`
			AppRole  any      `json:"app_role"`
			AppRole2 any      `json:"app-role"`
		}
		if err := token.Claims(&claims); err != nil {
			return nil, ErrInvalidToken
		}
		return &identity.Principal{
			Issuer:         token.Issuer,
			Subject:        token.Subject,
			Audience:       append([]string(nil), token.Audience...),
			Assurance:      strings.TrimSpace(claims.ACR),
			Methods:        append([]string(nil), claims.AMR...),
			ExpiresAt:      &token.Expiry,
			Groups:         extractClaimStringSet(claims.Groups, claims.Groups2),
			AppRoles:       extractClaimStringSet(claims.Roles, claims.AppRole, claims.AppRole2),
			IssuerVerified: true,
		}, nil
	}
	return nil, ErrInvalidToken
}
