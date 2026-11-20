package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// testOperatorFixture installs a verified operator identity for the duration of a
// test so the production CLI run path (runWithIO) authorizes a mutating
// operation in a managed environment. It builds the operator identity entirely
// in-test (a verified principal carrying an editor group plus a membership
// provider that maps it onto a single tenant editor membership); it never sets
// authentication, a token, a pin or an env from the lane or workflow
// (§3.8 letzter Absatz).
//
// The fixture mirrors the already-existing in-test operator pattern used by the
// operatorauth package (authorization_claims_test.go, authorization_test.go) and
// the cmd/baha operator_identity_transport_test.go / app_input_resolver_first_run_test.go
// fixtures, so the same contract the operator-auth tests assert is satisfied here
// without an operator OIDC configuration coming from the environment.
//
// It restores the previous hook through t.Cleanup so the package-global seam does
// not leak into sibling tests.
func testOperatorFixture(t *testing.T) {
	t.Helper()
	previous := testOperatorContext
	testOperatorContext = func(ctx context.Context) context.Context {
		principal := &identity.Principal{
			Subject:        "operator-fixture",
			Issuer:         "https://issuer.example",
			IssuerVerified: true,
			Groups:         []string{"editor"},
		}
		ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
		ctx = operatorauth.WithMembershipProvider(ctx, &testOperatorMembershipProvider{
			memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}},
		})
		return ctx
	}
	t.Cleanup(func() { testOperatorContext = previous })
}

// testOperatorMembershipProvider is the minimal bounded membership provider that
// maps the verified operator principal onto a single tenant editor membership,
// mirroring the shared operatorauth test stub.
type testOperatorMembershipProvider struct {
	memberships []tenancy.Membership
}

func (p *testOperatorMembershipProvider) ResolveClaimMembership(_ context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !principal.IssuerVerified {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return append([]tenancy.Membership(nil), p.memberships...), nil
}
