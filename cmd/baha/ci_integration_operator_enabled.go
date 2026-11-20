//go:build baseharbor_ci_operator_testonly

package main

import (
	"context"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// withCIIntegrationOperator exists only in binaries explicitly built with the
// baseharbor_ci_operator_testonly tag. It supplies the same synthetic verified
// operator used by Go tests, and only while the disposable demo acceptance gate
// is running. Release and ordinary CI binaries do not contain this path.
func withCIIntegrationOperator(ctx context.Context, args []string) context.Context {
	if os.Getenv("DEMO_ATOMIC_GATE") != "1" {
		return ctx
	}
	// Local Docker/Podman acceptance must exercise the real in-process runtime
	// grant. Inject the CI operator for an enrolled remote Core Target, or for
	// the initial target create in the isolated gate before that Target exists.
	// All subsequent local workload operations remain on the runtime-grant path.
	cfg, err := deployment.LoadConfig()
	if err != nil {
		if !ciIntegrationTargetCreate(args) {
			return ctx
		}
		return ciIntegrationOperatorContext(ctx)
	}
	target, err := cfg.ResolveTarget(strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), "")
	if err == nil && target.AccessProvider == "baseharbor-core" {
		return ciIntegrationOperatorContext(ctx)
	}
	// Bootstrap is the only local operation that may need a synthetic
	// operator: it must create the currently selected target, and that target
	// must not already resolve. Existing local workload operations therefore
	// continue through the runtime-grant authorization path.
	if err == nil || !ciIntegrationTargetCreate(args) {
		return ctx
	}
	return ciIntegrationOperatorContext(ctx)
}

func ciIntegrationTargetCreate(args []string) bool {
	selectedTarget := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
	return len(args) >= 3 && args[0] == "target" && args[1] == "create" &&
		selectedTarget != "" && strings.TrimSpace(args[2]) == selectedTarget
}

func ciIntegrationOperatorContext(ctx context.Context) context.Context {
	principal := &identity.Principal{
		Issuer: "https://issuer.test", Subject: "ci-acceptance-operator",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	return operatorauth.WithMembershipProvider(ctx, ciAcceptanceMembershipProvider{})
}

type ciAcceptanceMembershipProvider struct{}

func (ciAcceptanceMembershipProvider) ResolveClaimMembership(_ context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !operatorauth.PrincipalsHaveVerifiedIdentity(principal) {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return []tenancy.Membership{{TenantID: "tenant-ci-acceptance", Role: "editor"}}, nil
}
