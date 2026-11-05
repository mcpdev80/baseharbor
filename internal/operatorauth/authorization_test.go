package operatorauth

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestAuthorizeMachineOperationTrustedLocalAndManagedFailClosed(t *testing.T) {
	operation, ok := machine.OperationByID("apply")
	if !ok {
		t.Fatal("apply operation is not registered")
	}

	devCtx := WithEnforcement(context.Background())
	dev, err := AuthorizeMachineOperation(devCtx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "dev", Target: "local"},
	})
	if err != nil {
		t.Fatalf("dev authorization failed: %v", err)
	}
	if !dev.Allowed || dev.Actor.Mode != "trusted-local" || dev.Actor.Subject != "trusted-local" {
		t.Fatalf("unexpected dev decision: %#v", dev)
	}
	if dev.Safety != machine.SafetyMutating || !dev.PolicyRequired {
		t.Fatalf("machine safety metadata was not preserved: %#v", dev)
	}
	storedDev, ok := AuthorizationDecisionFromContext(devCtx)
	if !ok || storedDev.Actor.Subject != "trusted-local" || !storedDev.Allowed {
		t.Fatalf("trusted-local decision was not retained: %#v", storedDev)
	}

	managedCtx := WithEnforcement(context.Background())
	managed, err := AuthorizeMachineOperation(managedCtx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "prod", Target: "prod-eu"},
	})
	if err == nil {
		t.Fatal("managed authorization unexpectedly succeeded without principal")
	}
	if managed.Allowed || managed.ReasonCode != "operator_authentication_required" {
		t.Fatalf("managed decision did not fail closed: %#v", managed)
	}
	classified := machine.Classify(err)
	if classified.Code != machine.ErrorAuthenticationFailed {
		t.Fatalf("managed denial code = %q", classified.Code)
	}
	storedManaged, ok := AuthorizationDecisionFromContext(managedCtx)
	if !ok || storedManaged.Allowed || storedManaged.ReasonCode != "operator_authentication_required" {
		t.Fatalf("managed denial decision was not retained: %#v", storedManaged)
	}
}

func TestAuthorizeMachineOperationReturnsSecretSafeStableActor(t *testing.T) {
	operation, ok := machine.OperationByID("destroy")
	if !ok {
		t.Fatal("destroy operation is not registered")
	}

	ctx := WithEnforcement(context.Background())
	setPrincipal(ctx, &identity.Principal{
		Issuer:    "https://issuer.example",
		Subject:   "user-123",
		Audience:  []string{"baseharbor"},
		Assurance: "urn:mfa",
		Methods:   []string{"pwd", "otp"},
	})

	// A verified principal must still resolve a claim membership before it is
	// admitted; without an installed provider the verified actor is denied,
	// never fall-through allowed. Test the positive path with a bounded provider
	// and the fail-closed path without one.
	t.Run("admitted with provider membership", func(t *testing.T) {
		prov := claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}}
		ctxp := WithVerifiedPrincipal(ctx, &identity.Principal{
			Issuer: "https://issuer.example", Subject: "user-123", IssuerVerified: true,
			Groups: []string{"editor"}, AppRoles: []string{"db.admin"},
		})
		ctxp = WithMembershipProvider(ctxp, prov)
		dec, err := AuthorizeMachineOperation(ctxp, AuthorizationRequest{
			Operation: operation,
			Context:   OperationContext{Application: "demo", Environment: "prod", Target: "prod-eu"},
		})
		if err != nil {
			t.Fatalf("authorization failed with provider membership: %v", err)
		}
		if !dec.Allowed || dec.Actor.Mode != "authenticated" {
			t.Fatalf("unexpected decision: %#v", dec)
		}
		if dec.Actor.Issuer != "https://issuer.example" || dec.Actor.Subject != "user-123" {
			t.Fatalf("stable actor fields missing: %#v", dec.Actor)
		}
		stored, ok := AuthorizationDecisionFromContext(ctxp)
		if !ok || stored.Actor.Subject != "user-123" || stored.Actor.Issuer != "https://issuer.example" {
			t.Fatalf("authenticated decision was not retained: %#v", stored)
		}
	})
}

func TestVerifiedActorIsPreservedForDevelopmentEnvironment(t *testing.T) {
	operation, _ := machine.OperationByID("apply")
	// In `dev` the verified actor still retains the membership check; it does
	// not become a trusted-local operator. Without a resolved claim membership
	// it fails closed; with a provider membership it is admitted.
	t.Run("denied without provider membership", func(t *testing.T) {
		ctx := WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-a", IssuerVerified: true})
		decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{Operation: operation, Context: OperationContext{Environment: "dev", Target: "local"}})
		if err == nil || decision.Allowed {
			t.Fatal("verified actor without claim membership must fail closed, not be trusted-local", decision)
		}
		if machine.Classify(err).Code != machine.ErrorPolicyDenied {
			t.Fatal("dev verified actor without membership must policy-deny", err)
		}
	})
	t.Run("admitted with provider membership", func(t *testing.T) {
		ctx := WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-a", IssuerVerified: true, Groups: []string{"editor"}})
		ctx = WithMembershipProvider(ctx, claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}})
		decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{Operation: operation, Context: OperationContext{Environment: "dev", Target: "local"}})
		if err != nil || decision.Actor.Mode != "authenticated" || decision.Actor.Subject != "operator-a" {
			t.Fatal("verified HTTP/MCP actor with provider membership must be admitted", err)
		}
	})
}

func TestTenantMachinePermissionsFailClosed(t *testing.T) {
	for _, test := range []struct {
		name          string
		roles         []string
		safety        machine.SafetyClass
		tenantID      string
		identityID    string
		authenticated bool
		allowed       bool
	}{
		{name: "viewer read", roles: []string{"viewer"}, safety: machine.SafetyReadOnly, tenantID: "tenant-a", identityID: "identity-a", authenticated: true, allowed: true},
		{name: "viewer mutation", roles: []string{"viewer"}, safety: machine.SafetyMutating, tenantID: "tenant-a", identityID: "identity-a", authenticated: true},
		{name: "viewer destruction", roles: []string{"viewer"}, safety: machine.SafetyDestructive, tenantID: "tenant-a", identityID: "identity-a", authenticated: true},
		{name: "editor mutation", roles: []string{"editor"}, safety: machine.SafetyMutating, tenantID: "tenant-a", identityID: "identity-a", authenticated: true, allowed: true},
		{name: "editor destruction", roles: []string{"editor"}, safety: machine.SafetyDestructive, tenantID: "tenant-a", identityID: "identity-a", authenticated: true, allowed: true},
		{name: "unknown role", roles: []string{"admin"}, safety: machine.SafetyReadOnly, tenantID: "tenant-a", identityID: "identity-a", authenticated: true},
		{name: "no roles", safety: machine.SafetyReadOnly, tenantID: "tenant-a", identityID: "identity-a", authenticated: true},
		{name: "missing tenant", roles: []string{"editor"}, safety: machine.SafetyMutating, identityID: "identity-a", authenticated: true},
		{name: "missing membership", roles: []string{"editor"}, safety: machine.SafetyMutating, tenantID: "tenant-a", authenticated: true},
		{name: "unknown safety", roles: []string{"editor"}, tenantID: "tenant-a", identityID: "identity-a", authenticated: true},
		{name: "no authenticated actor in dev", roles: []string{"editor"}, safety: machine.SafetyMutating, tenantID: "tenant-a", identityID: "identity-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := tenancy.WithContext(context.Background(), &tenancy.Context{TenantID: test.tenantID, ExternalIdentityID: test.identityID, Roles: test.roles})
			ctx = WithEnforcement(ctx)
			if test.authenticated {
				setPrincipal(ctx, &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-a"})
			}
			decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
				Operation: machine.Operation{ID: "tenant-test", Safety: test.safety},
				Context:   OperationContext{Environment: "dev"},
			})
			if decision.Allowed != test.allowed || (err == nil) != test.allowed {
				t.Fatalf("decision = %#v, error = %v", decision, err)
			}
			stored, ok := AuthorizationDecisionFromContext(ctx)
			if !ok || stored.Allowed != test.allowed || stored.ReasonCode != decision.ReasonCode {
				t.Fatal("tenant authorization decision was not retained")
			}
			if !test.allowed && test.authenticated && (machine.Classify(err).Code != machine.ErrorPolicyDenied || decision.Actor.Subject != "operator-a") {
				t.Fatal("tenant denial must retain its actor and policy classification", err)
			}
		})
	}
}

func TestWithMembershipProviderIfAbsentPreservesExplicitProvider(t *testing.T) {
	principal := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "operator-explicit",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx := WithVerifiedPrincipal(context.Background(), principal)
	ctx = WithMembershipProvider(ctx, claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}})
	ctx = WithMembershipProviderIfAbsent(ctx, claimStub{})

	decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "prod", Target: "target-a"},
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("default production provider replaced the caller's explicit provider: %#v %v", decision, err)
	}
}
