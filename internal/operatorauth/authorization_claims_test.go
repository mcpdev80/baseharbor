package operatorauth

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// claimStub maps a verified principal's claim sets onto bounded memberships.
// It fails closed when the principal is not verified or carries no claim.
type claimStub struct {
	memberships []tenancy.Membership
	err         error
}

func (c claimStub) ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !principal.IssuerVerified {
		return nil, ErrClaimsNotVerified
	}
	if c.err != nil {
		return nil, c.err
	}
	if len(c.memberships) == 0 {
		return nil, ErrNoClaimMembership
	}
	return c.memberships, nil
}

func machineErrCause(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var mErr *machine.Error
	if errors.As(err, &mErr) {
		return mErr.CauseCode
	}
	return err.Error()
}

// TestClaimMappingAllowsWhenGroupMapsToPermission is the positive
// registry-fact proof: a valid issuer+subject with a mapped group/app-role
// claim is allowed on the machine operation.
func TestClaimMappingAllowsWhenGroupMapsToPermission(t *testing.T) {
	principal := &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "operator-1",
		IssuerVerified: true,
		Groups:         []string{"team-ops"},
		AppRoles:       []string{"db.admin"},
	}
	// db.admin claim resolves, via the bounded provider mapping, to the rbac
	// editor membership that carries the read permission.
	provider := claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}}

	ctx := context.Background()
	ctx = WithVerifiedPrincipal(ctx, principal)
	ctx = WithMembershipProvider(ctx, provider)

	dec, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{
			ID:             "app.show",
			Safety:         machine.SafetyReadOnly,
			PolicyRequired: true,
		},
		Context: OperationContext{Environment: "test", Target: "target-1"},
	})
	if err != nil {
		t.Fatalf("expected allowed for claim-mapped operation, got err: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected decision.Allowed=true for verified claim membership")
	}
	if dec.ReasonCode != "" {
		t.Fatalf("unexpected reason code: %q", dec.ReasonCode)
	}
}

// TestClaimMappingDeniesWithoutMembership proves the missing-group / no-membership
// fail-closed path: verified principal, but no claim resolves to a membership.
func TestClaimMappingDeniesWithoutMembership(t *testing.T) {
	principal := &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "operator-1",
		IssuerVerified: true,
		Groups:         []string{"team-visitor"},
	}
	provider := claimStub{memberships: nil}
	ctx := context.Background()
	ctx = WithVerifiedPrincipal(ctx, principal)
	ctx = WithMembershipProvider(ctx, provider)

	_, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	})
	if err == nil {
		t.Fatalf("expected deny when verified claim has no membership")
	}
	cause := machineErrCause(t, err)
	if cause == "" || cause == "unknown_operation" {
		t.Fatalf("expected a policy-deny cause, got %q", cause)
	}
}

// TestClaimMappingDeniesWhenClaimsNotVerified proves a non-verified principal
// never gets claim-based access.
func TestClaimMappingDeniesWhenClaimsNotVerified(t *testing.T) {
	principal := &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "operator-1",
		Groups:         []string{"team-ops"},
		IssuerVerified: false,
	}
	provider := claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}}
	ctx := context.Background()
	ctx = WithVerifiedPrincipal(ctx, principal)
	ctx = WithMembershipProvider(ctx, provider)

	_, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	})
	if err == nil {
		t.Fatalf("expected deny when claims are not verified")
	}
}

// TestClaimMappingDeniesScopeOverrun proves a mapped role that lacks the required
// permission denies (scope overrun fail-closed).
func TestClaimMappingDeniesScopeOverrun(t *testing.T) {
	principal := &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "operator-1",
		IssuerVerified: true,
		Groups:         []string{"team-viewer"},
	}
	// viewer role only has read; a mutating operation must be denied.
	provider := claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "viewer"}}}
	ctx := context.Background()
	ctx = WithVerifiedPrincipal(ctx, principal)
	ctx = WithMembershipProvider(ctx, provider)

	_, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.stop", Safety: machine.SafetyMutating, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	})
	if err == nil {
		t.Fatalf("expected scope-overrun deny for viewer on mutating operation")
	}
}
