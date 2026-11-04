package workstationenrollmenthttp

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/tenancy"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// policyAuthority authorizes a workstation-owned station label against the
// authenticated operator's installation. It answers only "is this station
// permitted for this tenant?"; it never derives the installation from the
// request. The authenticated installation is bound by tenancy in the resolver
// that supplies it.
type policyAuthority interface {
	// AuthorizeStation reports whether the workstation-owned station label is
	// authorized for the authenticated installation. An unowned, foreign, or
	// otherwise unauthorized station fails closed.
	AuthorizeStation(ctx context.Context, tenantID, station string) error
}

// scopeResolverFromPolicy is the canonical ScopeResolver for the production path.
// It binds the installation tenant from the authenticated operator (tenancy),
// never from the workstation request, and authorizes the workstation-owned
// station label against Core policy/ownership for that installation. A
// valid-looking but unowned station, or a foreign installation, fails closed
// before the store or issuer is touched.
func scopeResolverFromPolicy(ctx context.Context, station string, authorize policyAuthority) (ws.Scope, error) {
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant.TenantID == "" || tenant.ExternalIdentityID == "" {
		return ws.Scope{}, ws.ErrDenied
	}
	// Authorize the station against policy before binding any scope. An unowned
	// station fails here; a valid label for the authenticated installation passes.
	if authorize == nil || authorize.AuthorizeStation(ctx, tenant.TenantID, station) != nil {
		return ws.Scope{}, ws.ErrDenied
	}
	// Re-validate the label after authorization: authorization never returns a
	// malformed label, but a defense-in-depth guard keeps the scope canonical.
	scope := ws.Scope{TenantID: tenant.TenantID, Station: station}
	if scope.Validate() != nil {
		return ws.Scope{}, ws.ErrDenied
	}
	return scope, nil
}

// ScopeResolverFromPolicy returns the canonical production ScopeResolver. The
// installation tenant is bound by the authenticated operator (tenancy) and the
// workstation-owned station is authorized against Core policy/ownership, so the
// scope is never selected from the request. A foreign or unowned request fails
// closed.
func ScopeResolverFromPolicy(authorize policyAuthority) ScopeResolver {
	return func(ctx context.Context, station string) (ws.Scope, error) {
		return scopeResolverFromPolicy(ctx, station, authorize)
	}
}
