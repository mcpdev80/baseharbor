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
	principal := &identity.Principal{Issuer: "https://identity.example", Subject: "http-operator", Assurance: "verified", Methods: []string{"pwd"}}
	ctx := operatorauth.WithVerifiedPrincipal(withTargetOverride(context.Background(), target.Name), principal)
	for _, environment := range []string{"dev", "prod"} {
		result, err := inspectOperatorIdentity(ctx, environment)
		if err != nil || result.Actor.Mode != "authenticated" || result.Actor.Subject != principal.Subject || result.Target != target.Name {
			t.Fatal("verified transport discarded", result, err)
		}
	}
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant", ExternalIdentityID: "membership"})
	if _, err := inspectOperatorIdentity(ctx, "dev"); machine.Classify(err).Code != machine.ErrorPolicyDenied {
		t.Fatal("identity inspection bypassed tenant permission", err)
	}
}
