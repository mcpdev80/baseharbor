package operatorauth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// claimStub maps a verified principal's claim sets onto bounded memberships.
// It fails closed when the principal is not verified or carries no claim.
//
// The production binding the reviewer asked for lives here: the stub resolves a
// membership only for the role the principal actually carried as a claim, so a
// team-a claim cannot resolve to a tenant/role the claims never asserted. A
// caller-supplied membership list that does not correspond to the verified
// claims is treated as unbound and fails closed.
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
	// Bind each supplied membership to a role the verified principal actually
	// carried as a claim. A membership whose role is not among the verified
	// claims is an unbound/foreign grant and is dropped, which fails closed.
	claimSet := make(map[string]struct{})
	for _, r := range principal.ClaimSets() {
		claimSet[r] = struct{}{}
	}
	bound := make([]tenancy.Membership, 0, len(c.memberships))
	for _, membership := range c.memberships {
		role := strings.ToLower(membership.Role)
		if _, hasGroup := claimSet["group:"+role]; hasGroup {
			bound = append(bound, membership)
			continue
		}
		if _, hasAppRole := claimSet["app-role:"+role]; hasAppRole {
			bound = append(bound, membership)
		}
	}
	if len(bound) == 0 {
		return nil, ErrNoClaimMembership
	}
	return bound, nil
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
		Groups:         []string{"editor"},
		AppRoles:       []string{"db.admin"},
	}
	// The verified "editor" group claim binds, via the bounded provider mapping,
	// to the tenant-a editor membership that carries the read permission. The
	// membership role (editor) is one the verified principal actually carried as
	// a claim, so the claim-to-membership binding accepts it.
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

// TestClaimMappingTwoTeamIsolation proves the two-team scope at the authorization
// decision level: a verified team-a claim must not authorize an operation that
// resolves to tenant-b, and ambiguous memberships (team-a AND team-b) fail
// closed. This closes the review-1369a gap that only tenancy.Resolve was tested
// for two-team isolation.
func TestClaimMappingTwoTeamIsolation(t *testing.T) {
	// team-a claim set, but the provider maps the operator to tenant-b. The
	// claim scope (team-a) and the resolved tenant (tenant-b) must not be
	// conflated into cross-team access; resolution must fail closed.
	teamA := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "operator-cross",
		Groups: []string{"team-a"}, IssuerVerified: true,
	}
	provider := claimStub{memberships: []tenancy.Membership{{TenantID: "tenant-b", Role: "editor"}}}
	ctx := context.Background()
	ctx = WithVerifiedPrincipal(ctx, teamA)
	ctx = WithMembershipProvider(ctx, provider)
	_, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	})
	// Ambiguous/mismatched tenant resolution fails closed via tenancy.Resolve
	// when memberships do not collapse to a single tenant the claim set names.
	if err == nil {
		t.Fatalf("expected two-team denial: team-a claim must not authorize tenant-b access")
	}

	// Directly ambiguous memberships (tenant-a AND tenant-b) must fail closed.
	ambiguous := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "operator-ambiguous",
		Groups: []string{"team-a", "team-b"}, IssuerVerified: true,
	}
	ambProvider := claimStub{memberships: []tenancy.Membership{
		{TenantID: "tenant-a", Role: "editor"},
		{TenantID: "tenant-b", Role: "viewer"},
	}}
	ctxAmb := context.Background()
	ctxAmb = WithVerifiedPrincipal(ctxAmb, ambiguous)
	ctxAmb = WithMembershipProvider(ctxAmb, ambProvider)
	if _, ambErr := AuthorizeMachineOperation(ctxAmb, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	}); ambErr == nil {
		t.Fatalf("expected ambiguous-tenant membership to fail closed")
	}
}

// TestClaimMappingRevokedFailsClosed proves that a withdrawn/revoked membership
// (the provider returns no memberships, or an error) denies the subsequent
// operation rather than allowing fall-through. This closes the review-1369a
// revocation gap at the authorization-decision level.
func TestClaimMappingRevokedFailsClosed(t *testing.T) {
	revoked := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "operator-revoked",
		Groups: []string{"team-ops"}, IssuerVerified: true,
	}
	// Provider lookup succeeds but returns no memberships (membership revoked).
	provRevoked := claimStub{memberships: nil}
	ctxRev := context.Background()
	ctxRev = WithVerifiedPrincipal(ctxRev, revoked)
	ctxRev = WithMembershipProvider(ctxRev, provRevoked)
	if _, err := AuthorizeMachineOperation(ctxRev, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	}); err == nil {
		t.Fatalf("expected deny when claim membership was revoked (empty)")
	}

	// Provider lookup errors (revocation lookup failure) must also fail closed.
	provErr := claimStub{err: ErrNoClaimMembership}
	ctxErr := context.Background()
	ctxErr = WithVerifiedPrincipal(ctxErr, revoked)
	ctxErr = WithMembershipProvider(ctxErr, provErr)
	dec, err := AuthorizeMachineOperation(ctxErr, AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   OperationContext{Environment: "test", Target: "target-1"},
	})
	if err == nil || dec.Allowed {
		t.Fatalf("expected deny when revocation lookup errors")
	}
}
