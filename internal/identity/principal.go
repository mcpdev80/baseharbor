package identity

import (
	"context"
	"encoding/json"
	"fmt"
)

type contextKey string

const principalContextKey contextKey = "baseharbor:identity:principal"

// Principal is the provider-neutral authenticated identity used by BaseHarbor.
// Issuer + Subject form the stable external identity key.
type Principal struct {
	Issuer   string   `json:"issuer"`
	Subject  string   `json:"subject"`
	Audience []string `json:"audience,omitempty"`
}

// WithPrincipal stores an authenticated principal in a context.
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

// FromContext returns the authenticated principal when one is present.
func FromContext(ctx context.Context) (*Principal, bool) {
	value := ctx.Value(principalContextKey)
	principal, ok := value.(*Principal)
	return principal, ok && principal != nil
}

// String returns a logging-safe representation and never includes token material.
func (p *Principal) String() string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Principal{Issuer:%q, Subject:%q}", p.Issuer, p.Subject)
}

// JSON returns a serialization-safe representation of the principal.
func JSON(principal *Principal) string {
	if principal == nil {
		return "{}"
	}
	data, err := json.Marshal(principal)
	if err != nil {
		return "{}"
	}
	return string(data)
}
