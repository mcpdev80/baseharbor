package operatorauth

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const AuthorizationContractVersion = "v1"

type MachineActorRef struct {
	Mode      string   `json:"mode"`
	Issuer    string   `json:"issuer,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Assurance string   `json:"assurance,omitempty"`
	Methods   []string `json:"authentication_methods,omitempty"`
}

type OperationContext struct {
	Application string `json:"application,omitempty"`
	Environment string `json:"environment,omitempty"`
	Target      string `json:"target,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

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
		return decision, machine.NewError(machine.ErrorValidationFailed, decision.Reason, "Use a registered BaseHarbor machine operation.", false)
	}

	if !ManagedEnvironment(decision.Context.Environment) {
		decision.Allowed = true
		decision.Actor = MachineActorRef{Mode: "trusted-local", Subject: "trusted-local"}
		return decision, nil
	}

	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		decision.ReasonCode = "operator_authentication_required"
		decision.Reason = "Managed-environment machine operations require an authenticated BaseHarbor operator."
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
	return decision, nil
}
