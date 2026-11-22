// Test-only operator-auth fixture helpers exported for the CLI test surface.
// Card 0428-231 folds the verified-operator coverage that cmd/baha tests used
// to repeat call-site by call-site into the two exported symbols below, so the
// principal, the membership provider and the enforcement composition live on
// exactly one path. These helpers carry no production semantics: production
// composition never calls them, and they only assemble the existing context
// decorators of this package.
package operatorauth

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// ReadGrantingMembershipProvider is a bounded membership provider for tests of
// managed-environment paths that require an authenticated operator: it resolves
// any verified principal to a single tenant membership carrying the editor
// role, and it fails closed on a principal without a verified identity — the
// same contract every production provider must honour.
type ReadGrantingMembershipProvider struct{}

// ResolveClaimMembership grants exactly one editor membership for a verified
// principal and returns ErrClaimsNotVerified otherwise.
func (ReadGrantingMembershipProvider) ResolveClaimMembership(_ context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !PrincipalsHaveVerifiedIdentity(principal) {
		return nil, ErrClaimsNotVerified
	}
	return []tenancy.Membership{{TenantID: "tenant-local", Role: "editor"}}, nil
}

// WithVerifiedTestOperator installs a verified test-operator principal, a
// read-granting membership provider and enforcement on ctx, preserving a
// principal already supplied by an authenticated caller. The IfAbsent semantics
// mirror the package decorators exactly: an existing principal keeps its
// identity (replacing it would discard the caller's verified principal), while
// the membership provider is only installed when absent so an explicitly
// scoped provider is never overwritten.
func WithVerifiedTestOperator(ctx context.Context) context.Context {
	if _, ok := PrincipalFromContext(ctx); ok {
		ctx = WithMembershipProviderIfAbsent(ctx, ReadGrantingMembershipProvider{})
		return WithEnforcementIfAbsent(ctx)
	}
	ctx = WithVerifiedPrincipal(ctx, TestOperatorPrincipal())
	ctx = WithMembershipProviderIfAbsent(ctx, ReadGrantingMembershipProvider{})
	return WithEnforcementIfAbsent(ctx)
}

// WithVerifiedTestOperatorForce installs fresh enforcement state, the
// test-operator principal and a read-granting membership provider, mirroring
// the previous runWithIOAndOperator composition whose WithEnforcement replaced
// any prior enforcement. Callers that need per-invocation interactive or
// decision state compose the existing decorators after this helper.
func WithVerifiedTestOperatorForce(ctx context.Context) context.Context {
	ctx = WithEnforcement(ctx)
	ctx = WithVerifiedPrincipal(ctx, TestOperatorPrincipal())
	return WithMembershipProvider(ctx, ReadGrantingMembershipProvider{})
}

// TestOperatorPrincipal returns the canonical verified editor principal the
// CLI test surface used to build inline at every call site.
func TestOperatorPrincipal() *identity.Principal {
	return &identity.Principal{
		Issuer:         "https://issuer.test",
		Subject:        "cli-test-operator",
		IssuerVerified: true,
		Groups:         []string{"editor"},
	}
}
