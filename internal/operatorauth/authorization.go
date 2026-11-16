package operatorauth

import (
	"context"
	"errors"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// ErrClaimsNotVerified is returned when a claim-based tenant resolution is
// attempted against a principal whose claims were not produced by a verified
// OIDC/JWT token. Claim-derived roles never grant access on their own.
var ErrClaimsNotVerified = errors.New("operator claims are not from a verified token")

// ErrNoClaimMembership is returned when a verified principal carries no group or
// app-role claim that maps to a tenant membership. Missing membership fails
// closed; it is not a configuration error.
var ErrNoClaimMembership = errors.New("verified operator carries no claim-based tenant membership")

// MembershipProvider resolves the bounded tenant memberships for one verified
// operator identity. Implementations map the verified issuer+subject (and the
// verified group/app-role claims carried by the principal) to concrete
// tenant/role memberships. It must fail closed on a non-verified principal and
// return ErrNoClaimMembership when the identity holds no membership. The
// production implementation (database-backed memberships) is the authority; this
// package only bridges the verified claim sets onto that boundary.
type MembershipProvider interface {
	ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error)
}

// resolveClaimMembershipToTenant returns a single-tenant scope derived from the
// verified claim sets, or an error. It requires an authenticated principal, an
// explicit MembershipProvider and verified claims; without all three it fails
// closed so claim-derived access can never appear without a real provider.
//
// The resolved tenant memberships must be bound to the verified claim sets: a
// team-a claim must resolve to a team-a membership, never a foreign tenant. This
// is what makes the two-team boundary enforce at the authorization-decision
// level rather than only in tenancy.Resolve.
func resolveClaimMembershipToTenant(ctx context.Context, principal *identity.Principal, provider MembershipProvider) (*tenancy.Context, error) {
	if provider == nil || principal == nil || !principal.IssuerVerified {
		return nil, ErrClaimsNotVerified
	}
	memberships, err := provider.ResolveClaimMembership(ctx, principal)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, ErrNoClaimMembership
	}
	scoped, err := boundClaimsToMemberships(principal, memberships)
	if err != nil {
		return nil, err
	}
	return tenancy.Resolve(principal.Subject, scoped)
}

// boundClaimsToMemberships verifies that the resolved memberships are bound to
// the principal's verified claim sets: every returned membership's role must
// appear in the principal's claim sets, and the membership must resolve to a
// single tenant that the verified claims name. A membership whose role is not
// among the verified claims, or an ambiguous multi-tenant resolution, fails
// closed so a team-a claim can never resolve to a tenant the claims do not
// authorize. This is the two-team isolation check at the authorization boundary.
func boundClaimsToMemberships(principal *identity.Principal, memberships []tenancy.Membership) ([]tenancy.Membership, error) {
	claimSet := make(map[string]struct{})
	for _, c := range principal.ClaimSets() {
		claimSet[c] = struct{}{}
	}
	roleClaimed := func(role string) bool {
		role = strings.ToLower(role)
		_, hasGroup := claimSet["group:"+role]
		_, hasAppRole := claimSet["app-role:"+role]
		return hasGroup || hasAppRole
	}
	var tenantID string
	bound := make([]tenancy.Membership, 0, len(memberships))
	for _, membership := range memberships {
		if membership.TenantID == "" || membership.Role == "" {
			return nil, tenancy.ErrInvalidMembership
		}
		// The role on the membership must be one the verified principal actually
		// carried as a claim. An unclaimed role (e.g. a foreign tenant granting a
		// role the team-a principal never asserted) fails closed.
		if !roleClaimed(membership.Role) {
			return nil, ErrClaimsNotVerified
		}
		if tenantID == "" {
			tenantID = membership.TenantID
		} else if membership.TenantID != tenantID {
			return nil, tenancy.ErrAmbiguousTenant
		}
		bound = append(bound, membership)
	}
	if len(bound) == 0 {
		return nil, ErrNoClaimMembership
	}
	return bound, nil
}

const AuthorizationContractVersion = "v1"

type MachineActorRef = machine.ActorRef

type OperationContext = machine.OperationContext

type AuthorizationRequest struct {
	Operation machine.Operation
	Context   OperationContext
}

type AuthorizationDecision struct {
	ContractVersion      string              `json:"contract_version"`
	Allowed              bool                `json:"allowed"`
	Operation            string              `json:"operation"`
	Safety               machine.SafetyClass `json:"safety"`
	PolicyRequired       bool                `json:"policy_required"`
	ConfirmationRequired bool                `json:"confirmation_required"`
	Actor                MachineActorRef     `json:"actor"`
	Context              OperationContext    `json:"context"`
	ReasonCode           string              `json:"reason_code,omitempty"`
	Reason               string              `json:"reason,omitempty"`
}

func AuthorizeMachineOperation(ctx context.Context, request AuthorizationRequest) (AuthorizationDecision, error) {
	operation := request.Operation
	decision := AuthorizationDecision{
		ContractVersion:      AuthorizationContractVersion,
		Operation:            operation.ID,
		Safety:               operation.Safety,
		PolicyRequired:       operation.PolicyRequired,
		ConfirmationRequired: operation.ConfirmationRequired,
		Context: OperationContext{
			Application: strings.TrimSpace(request.Context.Application),
			Environment: strings.ToLower(strings.TrimSpace(request.Context.Environment)),
			Target:      strings.TrimSpace(request.Context.Target),
			Workspace:   strings.TrimSpace(request.Context.Workspace),
		},
	}

	if strings.TrimSpace(operation.ID) == "" {
		decision.ReasonCode = "unknown_operation"
		decision.Reason = "Machine operation metadata is required for authorization."
		setAuthorizationDecision(ctx, decision)
		return decision, machine.NewError(machine.ErrorValidationFailed, decision.Reason, "Use a registered BaseHarbor machine operation.", false)
	}

	principal, ok := PrincipalFromContext(ctx)
	// An imported workstation grants the minimum development-only device
	// workflow, never production or staging by virtue of carrying a valid
	// client certificate. Elevated environments require an operator OIDC
	// session and explicit policy instead of an enrollment invite.
	if ok && principal != nil {
		for _, method := range principal.Methods {
			if method == "workstation-mtls" && decision.Context.Environment != "dev" {
				decision.ReasonCode = "workstation_environment_denied"
				decision.Reason = "Workstation pairing alone permits only development environment operations."
				setAuthorizationDecision(ctx, decision)
				return decision, &machine.Error{
					Code: machine.ErrorPolicyDenied, CauseCode: decision.ReasonCode,
					Message: decision.Reason, Resource: operation.ID,
					Next: "Authenticate an operator and request elevated environment access explicitly.",
				}
			}
		}
	}
	tenantScoped := false
	if tenant, scoped := tenancy.FromContext(ctx); scoped {
		if !ok {
			decision.ReasonCode = "operator_authentication_required"
			decision.Reason = "Tenant-scoped machine operations require an authenticated operator."
			setAuthorizationDecision(ctx, decision)
			return decision, machine.NewError(machine.ErrorAuthenticationFailed, decision.Reason, "Authenticate with the configured OIDC provider.", false)
		}
		decision.Actor = MachineActorRef{
			Mode: "authenticated", Issuer: strings.TrimSpace(principal.Issuer),
			Subject: strings.TrimSpace(principal.Subject), Assurance: strings.TrimSpace(principal.Assurance),
			Methods: append([]string(nil), principal.Methods...),
		}
		permission := machineOperationPermission(operation.Safety)
		if strings.TrimSpace(tenant.TenantID) == "" || strings.TrimSpace(tenant.ExternalIdentityID) == "" ||
			!authorization.NewService().Allowed(tenant.Roles, permission) {
			decision.ReasonCode = "tenant_permission_denied"
			decision.Reason = "The resolved tenant membership does not permit this machine operation."
			setAuthorizationDecision(ctx, decision)
			return decision, &machine.Error{
				Code: machine.ErrorPolicyDenied, CauseCode: decision.ReasonCode,
				Message: decision.Reason, Resource: operation.ID,
				Next: "Use a tenant membership with the required Core permission.",
			}
		}
		tenantScoped = true
	}
	// No tenant context yet: bridge the verified issuer+subject/group/app-role
	// claims onto the bounded membership boundary. A verified principal in a
	// claim-scoped operation MUST resolve a claim membership; missing
	// membership, an unverified principal, a failed provider lookup or an
	// ambiguous membership all fail closed. This fails closed rather than
	// falling through to the trusted-local fallback, because a verified
	// operator identity alone must never grant a machine operation without a
	// current claim-to-membership result.
	if !tenantScoped && ok {
		provider := membershipProviderFromContext(ctx)
		resolved, err := resolveClaimMembershipToTenant(ctx, principal, provider)
		if err != nil || resolved == nil {
			decision.ReasonCode = "claim_membership_unresolved"
			decision.Reason = "The verified operator carried no claim-based tenant membership that permits this machine operation."
			setAuthorizationDecision(ctx, decision)
			return decision, &machine.Error{
				Code: machine.ErrorPolicyDenied, CauseCode: decision.ReasonCode,
				Message: decision.Reason, Resource: operation.ID,
				Next: "Authenticate with a tenant membership that maps the operator's claims.",
			}
		}
		ctx = tenancy.WithContext(ctx, resolved)
		tenantScoped = true
		decision.Actor = MachineActorRef{
			Mode: "authenticated", Issuer: strings.TrimSpace(principal.Issuer),
			Subject: strings.TrimSpace(principal.Subject), Assurance: strings.TrimSpace(principal.Assurance),
			Methods: append([]string(nil), principal.Methods...),
		}
		permission := machineOperationPermission(operation.Safety)
		if !authorization.NewService().Allowed(resolved.Roles, permission) {
			decision.ReasonCode = "tenant_permission_denied"
			decision.Reason = "The claim-based tenant membership does not permit this machine operation."
			setAuthorizationDecision(ctx, decision)
			return decision, &machine.Error{
				Code: machine.ErrorPolicyDenied, CauseCode: decision.ReasonCode,
				Message: decision.Reason, Resource: operation.ID,
				Next: "Use a tenant membership with the required Core permission.",
			}
		}
		decision.Allowed = true
		setAuthorizationDecision(ctx, decision)
		return decision, nil
	}
	if !ok && localRuntimeAuthorityMatches(ctx, request) {
		decision.Allowed = true
		decision.Actor = MachineActorRef{Mode: "local-runtime", Subject: "local-runtime/" + decision.Context.Target}
		setAuthorizationDecision(ctx, decision)
		return decision, nil
	}
	if !ManagedEnvironment(decision.Context.Environment) && !ok {
		decision.Allowed = true
		decision.Actor = MachineActorRef{Mode: "trusted-local", Subject: "trusted-local"}
		setAuthorizationDecision(ctx, decision)
		return decision, nil
	}

	if !ok {
		decision.ReasonCode = "operator_authentication_required"
		decision.Reason = "Managed-environment machine operations require an authenticated BaseHarbor operator."
		setAuthorizationDecision(ctx, decision)
		return decision, machine.NewError(
			machine.ErrorAuthenticationFailed,
			decision.Reason,
			"Authenticate the operator for the selected Target and environment, then retry the same operation.",
			false,
		)
	}

	decision.Allowed = true
	decision.Actor = MachineActorRef{
		Mode:      "authenticated",
		Issuer:    strings.TrimSpace(principal.Issuer),
		Subject:   strings.TrimSpace(principal.Subject),
		Assurance: strings.TrimSpace(principal.Assurance),
		Methods:   append([]string(nil), principal.Methods...),
	}
	setAuthorizationDecision(ctx, decision)
	return decision, nil
}

func machineOperationPermission(safety machine.SafetyClass) authorization.Permission {
	switch safety {
	case machine.SafetyReadOnly:
		return authorization.PermRead
	case machine.SafetyMutating:
		return authorization.PermUpdate
	case machine.SafetyDestructive:
		return authorization.PermDelete
	default:
		return ""
	}
}
