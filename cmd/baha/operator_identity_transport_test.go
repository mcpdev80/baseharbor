package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestOperatorIdentityUsesVerifiedTransportInsteadOfLocalSession(t *testing.T) {
	target := configureTestTarget(t)
	principal := &identity.Principal{Issuer: "https://identity.example", Subject: "http-operator", Assurance: "verified", Methods: []string{"pwd"}, IssuerVerified: true, Groups: []string{"editor"}}
	ctx := operatorauth.WithVerifiedPrincipal(withTargetOverride(context.Background(), target.Name), principal)
	ctx = operatorauth.WithMembershipProvider(ctx, operatorauthClaimStubForCmd{})
	// A verified transport is preserved (not collapsed to trusted-local) once it
	// resolves a claim membership. dev no longer auto-admits a verified identity;
	// it retains the membership check.
	for _, environment := range []string{"dev", "prod"} {
		result, err := inspectOperatorIdentity(ctx, environment)
		if err != nil || result.Actor.Mode != "authenticated" || result.Actor.Subject != principal.Subject || result.Target != target.Name {
			t.Fatal("verified transport discarded", result, err)
		}
	}
	// A verified operator with no resolved claim membership fails closed.
	noopCtx := operatorauth.WithVerifiedPrincipal(withTargetOverride(context.Background(), target.Name), &identity.Principal{Issuer: "https://identity.example", Subject: "http-operator", Assurance: "verified", Methods: []string{"pwd"}, IssuerVerified: true})
	if _, err := inspectOperatorIdentity(noopCtx, "prod"); machine.Classify(err).Code != machine.ErrorPolicyDenied {
		t.Fatal("verified operator without claim membership must policy-deny", err)
	}
	// Identity inspection must still bypass tenant permission only when a tenant
	// context carries a matching membership; without one it policy-denies.
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant", ExternalIdentityID: "membership"})
	if _, err := inspectOperatorIdentity(ctx, "dev"); machine.Classify(err).Code != machine.ErrorPolicyDenied {
		t.Fatal("identity inspection bypassed tenant permission", err)
	}
}

// operatorauthClaimStubForCmd is the minimal bounded provider binding used by the
// cmd/baha operator-identity tests: it maps the verified principal onto a single
// tenant editor membership so the claim-to-membership boundary admits it, mirroring
// the shared operatorauth test stub.
type operatorauthClaimStubForCmd struct{}

func (operatorauthClaimStubForCmd) ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !principal.IssuerVerified {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}, nil
}
