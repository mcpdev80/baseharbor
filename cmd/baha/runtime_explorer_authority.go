package main

import (
	"context"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

type runtimeMutationAuthority struct{}

func (runtimeMutationAuthority) AuthorizeRuntimeMutation(ctx context.Context, resource runtimeexplorer.Resource, request runtimeexplorer.OperationRequest) (runtimeexplorer.MutationVerifier, error) {
	plan, err := planRuntimeMutation(resource, request.Operation)
	if err != nil {
		return nil, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return nil, err
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, ack); err != nil {
		return nil, runtimeMutationPlanError(plan, err)
	}
	current, err := planRuntimeMutation(resource, request.Operation)
	if err != nil {
		return nil, err
	}
	if current.Scope.PlanID != plan.Scope.PlanID {
		return nil, runtimeMutationPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return func(ctx context.Context, actual runtimeexplorer.Resource, mutationErr error) (*application.ProviderImpactScopeResult, error) {
		post, observeErr := planRuntimeMutation(actual, request.Operation)
		owner := application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "unknown", Evidence: "Post-operation recovery ownership is unverified."}
		if observeErr == nil && post.storageRevision == plan.storageRevision {
			owner.Disposition = "retained"
			owner.Verified = true
			owner.Evidence = "Canonical deployment/provider recovery owner bytes are unchanged."
		}
		result, err := application.RecordProviderImpactScopeResult(plan.Scope, owner, nil)
		valid := actual.Ref == resource.Ref && actual.Ownership == resource.Ownership && actual.Relationship == resource.Relationship && actual.State.Ready != nil
		if request.Operation == runtimeexplorer.OperationStop {
			valid = valid && !*actual.State.Ready && (actual.State.Observed == "exited" || actual.State.Observed == "stopped" || actual.State.Observed == "created")
		}
		if request.Operation != runtimeexplorer.OperationStop {
			valid = valid && *actual.State.Ready
		}
		if mutationErr != nil || observeErr != nil || err != nil || !owner.Verified || !valid {
			return &result, &machine.Error{Code: machine.ErrorConflict, CauseCode: "runtime_mutation_partial", Message: "Runtime operation is incomplete; inspect the actual state and retained owner before retrying.", Details: struct {
				Plan   runtimeMutationPlan                   `json:"plan"`
				Result application.ProviderImpactScopeResult `json:"result"`
				Actual runtimeexplorer.Resource              `json:"actual"`
			}{plan, result, actual}}
		}
		return &result, nil
	}, nil
}

func runtimeMutationOwnerError() error {
	return machine.NewError(machine.ErrorOwnershipAmbiguous, "Runtime mutation requires verified existing Target and deployment ownership.", "Inspect the protected owner and native resource before retrying; no ownership is inferred.", false)
}

func runtimeMutationPlanError(plan runtimeMutationPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if !errors.As(classified, &failure) {
		failure = machine.NewError(machine.ErrorConflict, "Runtime consequence plan changed.", "Review the current plan and explicitly acknowledge it again.", false)
	}
	failure.Details = plan
	return failure
}
