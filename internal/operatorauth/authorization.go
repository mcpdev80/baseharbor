package operatorauth

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

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
