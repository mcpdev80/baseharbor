package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type contextKey string

const principalContextKey contextKey = "baseharbor:identity:principal"

// Principal is the provider-neutral authenticated identity used by BaseHarbor.
// Issuer + Subject form the stable external identity key.
//
// Groups and AppRoles are provider-neutral claim sets extracted from a verified
// OIDC/JWT id token. They feed the bounded claim/policy chain that maps an
// authenticated identity onto Target/environment/resource-operation permissions;
// they never grant a right on their own and are only meaningful when the
// token that produced them verified against its issuer.
type Principal struct {
	Issuer         string     `json:"issuer"`
	Subject        string     `json:"subject"`
	Audience       []string   `json:"audience,omitempty"`
	Assurance      string     `json:"assurance,omitempty"`
	Methods        []string   `json:"authentication_methods,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Groups         []string   `json:"groups,omitempty"`
	AppRoles       []string   `json:"app_roles,omitempty"`
	IssuerVerified bool       `json:"issuer_verified"`
}

// ClaimSets returns the claim-derived role input as a single sorted,
// de-duplicated slice. Groups and AppRoles are namespaced so a group named the
// same as an app-role cannot be confused, and an empty principal yields no
// roles. This is the bounded view the policy chain consumes.
func (p *Principal) ClaimSets() []string {
	if p == nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, g := range p.Groups {
		if s := strings.TrimSpace(g); s != "" {
			set["group:"+strings.ToLower(s)] = struct{}{}
		}
	}
	for _, r := range p.AppRoles {
		if s := strings.TrimSpace(r); s != "" {
			set["app-role:"+strings.ToLower(s)] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	roles := make([]string, 0, len(set))
	for role := range set {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
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
