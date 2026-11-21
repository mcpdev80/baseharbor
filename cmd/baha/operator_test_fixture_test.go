package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// init installs the whole-package operator-auth seam for the cmd/baha test run.
// #1594 (develop head 40cb28c75) removed the dev→trusted-local auto-admit, so
// every cmd/baha test that drives the production CLI run path (runWithIO, a
// direct command .Run(ctx), or an MCP tool handler) in a managed environment now
// fails with "Managed-environment machine operations require an authenticated
// BaseHarbor operator." This seam stamps a verified operator principal carrying
// the editor group onto the context the runWithIO seam touches, so the whole
// test package authorizes in-test without any workflow/owner OIDC configuration.
//
// Production leaves testOperatorContext nil; only the test binary sets it once.
// It is a single permanent seam rather than a per-test t.Cleanup hook because
// the failing cells build their contexts at many independent entry points and no
// single per-test helper can reach them all (see operatorAuthFixtureContext).
func init() {
	testOperatorContext = func(ctx context.Context) context.Context {
		return operatorAuthFixtureContext(ctx)
	}
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

// operatorAuthFixtureContext is the single shared collection point for the
// 0428-198 gate map: it stamps the in-test operator identity onto a context in
// the exact production order the runWithIO seam documents — WithVerifiedPrincipal
// carrying an editor group, then a membership provider that maps that principal
// onto one tenant editor membership. Every context builder in the cmd/baha test
// package that drives a production command in a managed environment routes its
// base through this helper instead of building an unauthenticated context: this
// one helper replaces the dozens of individually wired failing cells that
// #1594's dev→trusted-local removal broke. Production never calls it.
//
// The order matters: WithVerifiedPrincipal stores the principal in the
// enforcement state, so a later WithEnforcement would wipe it; callers that
// still need WithEnforcement apply it first (e.g. persesMatrixContext), then
// this helper. This mirrors the canonical example in
// cmd/baha/app_input_resolver_first_run_test.go:21-27 and the runWithIO seam in
// cmd/baha/main.go.
func operatorAuthFixtureContext(ctx context.Context) context.Context {
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

// operatorAuthTestContext supplies the fixture's public OIDC configuration as
// test-local environment and binds its verified principal to the operation
// context. Tests that intentionally exercise unauthenticated denial keep using
// the unstamped context.
func operatorAuthTestContext(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	t.Setenv(operatorauth.EnvIssuer, "https://issuer.example")
	t.Setenv(operatorauth.EnvClientID, "baseharbor-test")
	return operatorAuthFixtureContext(ctx)
}
