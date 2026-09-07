package tenancy

import (
	"context"
	"errors"
	"sort"
)

type contextKey string

const tenantContextKey contextKey = "baseharbor:tenancy:context"

var (
	ErrNoMembership       = errors.New("no tenant membership")
	ErrAmbiguousTenant    = errors.New("ambiguous tenant membership")
	ErrInvalidMembership  = errors.New("invalid tenant membership")
)

// Context is the resolved tenant scope for one authenticated request.
type Context struct {
	TenantID           string
	ExternalIdentityID string
	Roles              []string
}

// Membership is the provider-neutral membership input used during resolution.
type Membership struct {
	TenantID string
	Role     string
}

// WithContext stores a resolved tenant context in a context.Context.
func WithContext(ctx context.Context, tenant *Context) context.Context {
	return context.WithValue(ctx, tenantContextKey, tenant)
}

// FromContext returns the resolved tenant scope when present.
func FromContext(ctx context.Context) (*Context, bool) {
	value := ctx.Value(tenantContextKey)
	tenant, ok := value.(*Context)
	return tenant, ok && tenant != nil
}

// Resolve converts memberships for one external identity into a single tenant scope.
// Multiple memberships are allowed only when they all belong to the same tenant.
// Roles are deduplicated and sorted. Any ambiguity fails closed.
func Resolve(externalIdentityID string, memberships []Membership) (*Context, error) {
	if externalIdentityID == "" {
		return nil, ErrInvalidMembership
	}
	if len(memberships) == 0 {
		return nil, ErrNoMembership
	}

	var tenantID string
	roleSet := make(map[string]struct{}, len(memberships))

	for _, membership := range memberships {
		if membership.TenantID == "" || membership.Role == "" {
			return nil, ErrInvalidMembership
		}
		if tenantID == "" {
			tenantID = membership.TenantID
		} else if membership.TenantID != tenantID {
			return nil, ErrAmbiguousTenant
		}
		roleSet[membership.Role] = struct{}{}
	}

	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	return &Context{
		TenantID:           tenantID,
		ExternalIdentityID: externalIdentityID,
		Roles:              roles,
	}, nil
}
