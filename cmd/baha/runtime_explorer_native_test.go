package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

// Invoked only inside the existing explicitly enabled, isolated native gate.
// Source-only tests never substitute a native backend or ownership observation.
func qualifyActualRuntimeExplorerConsent(t *testing.T, ctx context.Context, resolved resolvedApplication) {
	t.Helper()
	explorer, target, err := runtimeExplorerForTarget(ctx, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := explorer.List(ctx, runtimeexplorer.ListRequest{Target: target, ApplicationID: resolved.Manifest.ApplicationID})
	if err != nil || len(resources) != 1 || resources[0].Ownership != runtimeexplorer.OwnershipManaged {
		t.Fatalf("actual native workload lacks canonical explorer ownership: %+v %v", resources, err)
	}
	resource := resources[0]
	input := machineRuntimeMutationInput{Target: target, Environment: resolved.Manifest.Environment, Provider: resource.Ref.Provider, Kind: string(resource.Ref.Kind), ResourceID: resource.Ref.ResourceID}
	_, err = executeRuntimeMutation(ctx, input, runtimeexplorer.OperationStop)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("ordinary runtime stop bypassed impact acknowledgement: %v", err)
	}
	after, err := explorer.Inspect(ctx, resource.Ref)
	if err != nil || after.State.Ready == nil || !*after.State.Ready {
		t.Fatal("denied stop changed actual workload")
	}
	for _, operation := range []runtimeexplorer.Operation{runtimeexplorer.OperationStop, runtimeexplorer.OperationStart, runtimeexplorer.OperationRestart} {
		current, err := explorer.Inspect(ctx, resource.Ref)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := planRuntimeMutation(current, operation)
		if err != nil {
			t.Fatal(err)
		}
		input.ProviderChangeConsent = &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
		var result runtimeexplorer.OperationResult
		if operation == runtimeexplorer.OperationStart {
			result, err = executeRuntimeOperation(ctx, machineRuntimeOperateInput{Target: input.Target, Environment: input.Environment, Provider: input.Provider, Kind: input.Kind, ResourceID: input.ResourceID, Operation: string(operation), ProviderChangeConsent: input.ProviderChangeConsent})
		} else {
			result, err = executeRuntimeMutation(ctx, input, operation)
		}
		if err != nil || result.ProviderChangeResult == nil || result.ProviderChangeResult.Status != "partial" || !result.ProviderChangeResult.Owner.Verified {
			t.Fatalf("actual acknowledged operation=%s: %+v %v", operation, result, err)
		}
		t.Logf("Actual native Runtime Explorer %s: state=%s, same resource ID, unchanged recovery owner, consequence receipt partial (function/transient effects unverified)", operation, result.State.Observed)
	}
}
