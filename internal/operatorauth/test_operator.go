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
	return WithVerifiedTestOperatorPrincipal(ctx, TestOperatorPrincipal())
}

// WithVerifiedTestOperatorPrincipalForced is the forced-compose entry point for
// call sites whose previous order enabled enforcement, installed a verified
// principal and then the read-granting membership provider (the
// WithEnforcement here replaces any prior enforcement, mirroring those
// compositions exactly). The supplied principal owns the identity; an empty
// principal means the canonical test operator.
func WithVerifiedTestOperatorPrincipalForced(ctx context.Context, principal *identity.Principal) context.Context {
	if principal == nil {
		principal = TestOperatorPrincipal()
	}
	ctx = WithEnforcement(ctx)
	ctx = WithVerifiedPrincipal(ctx, principal)
	return WithMembershipProvider(ctx, ReadGrantingMembershipProvider{})
}

// WithVerifiedTestOperatorPrincipalIfAbsent is the if-absent entry point for
// call sites whose previous order installed the principal FIRST (via
// WithVerifiedPrincipal, whose enforcement step is if-absent) and only then
// enabled enforcement and the membership provider. It preserves a principal
// already supplied by an authenticated caller, like WithVerifiedTestOperator.
func WithVerifiedTestOperatorPrincipalIfAbsent(ctx context.Context, principal *identity.Principal) context.Context {
	if principal == nil {
		principal = TestOperatorPrincipal()
	}
	if _, ok := PrincipalFromContext(ctx); ok {
		ctx = WithMembershipProviderIfAbsent(ctx, ReadGrantingMembershipProvider{})
		return WithEnforcementIfAbsent(ctx)
	}
	ctx = WithVerifiedPrincipal(ctx, principal)
	ctx = WithMembershipProviderIfAbsent(ctx, ReadGrantingMembershipProvider{})
	return WithEnforcementIfAbsent(ctx)
}

// WithVerifiedTestOperatorPrincipal is the single composition entry point: it
// installs enforcement, the membership provider and the CALLER-SUPPLIED
// verified principal on exactly one path (if-absent semantics: an existing
// principal keeps its identity; an explicitly scoped provider is never
// overwritten). Test call sites that used to repeat the manual
// WithEnforcement/WithVerifiedPrincipal/WithMembershipProvider composition
// with their own fixture identity route through this one helper.
func WithVerifiedTestOperatorPrincipal(ctx context.Context, principal *identity.Principal) context.Context {
	return WithVerifiedTestOperatorPrincipalIfAbsent(ctx, principal)
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

// TestPrincipal builds a verified editor principal for the CLI test surface
// with the given issuer and subject, so fixture identities (which tests used
// to inline at each call site) ride the same one composition path as the
// canonical operator. Empty arguments fall back to the canonical values.
func TestPrincipal(issuer, subject string) *identity.Principal {
	if issuer == "" {
		issuer = "https://issuer.test"
	}
	if subject == "" {
		subject = "cli-test-operator"
	}
	return &identity.Principal{
		Issuer:         issuer,
		Subject:        subject,
		IssuerVerified: true,
		Groups:         []string{"editor"},
	}
}

// TestOperatorPrincipal returns the canonical verified editor principal the
// CLI test surface used to build inline at every call site.
func TestOperatorPrincipal() *identity.Principal {
	return TestPrincipal("", "")
}
