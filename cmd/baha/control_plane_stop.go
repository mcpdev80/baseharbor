package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type controlPlaneStopResult struct {
	Target               string                                `json:"target"`
	State                string                                `json:"state"`
	Runtime              bhruntime.RuntimeServiceStopResult    `json:"runtime"`
	ProviderChangeResult application.ProviderImpactScopeResult `json:"provider_change_result"`
}

func runtimeDown(parent context.Context, out io.Writer) error {
	_, err := runtimeDownWithResult(parent, out)
	return err
}

func runtimeDownWithResult(parent context.Context, out io.Writer) (controlPlaneStopResult, error) {
	result := controlPlaneStopResult{State: "unknown"}
	if err := authorizeCurrentMCPContext(parent, "control-plane.stop", "", "", ""); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	target, err := effectiveTarget(ctx)
	if err != nil {
		return result, err
	}
	if err := authorizeCurrentMCPContext(ctx, "control-plane.stop", target.Name, "", ""); err != nil {
		return result, err
	}
	root, err := targetDataRoot(target)
	if err != nil {
		return result, err
	}
	unlock, err := lockApplicationTargetLifecycle(resolvedApplication{Target: target, TargetStateRoot: root})
	if err != nil {
		return result, err
	}
	defer unlock()
	plan, err := authorizeControlPlaneStop(ctx, out)
	if err != nil {
		return result, err
	}
	if plan.targetBinding != target {
		return result, controlPlaneStopPlanError(plan, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return result, err
	}
	current, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		return result, err
	}
	if current.Scope.PlanID != plan.Scope.PlanID {
		return result, controlPlaneStopPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	environment, err := bhruntime.RuntimeEnvironment(plan.inventory.Files)
	if err != nil {
		return result, runtimeMutationOwnerError()
	}
	result.Target = target.Name
	result.Runtime, err = bhruntime.StopOwnedProjectServices(ctx, provider, plan.Project, filepath.Dir(plan.inventory.Files.Compose), environment, plan.Resources, plan.Services, plan.inventory.Files.Compose)
	// Never translate command success into retained recovery authority. Every
	// protected input that was acknowledged must still have its original bytes.
	owner := application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "unknown", Evidence: "Recovery owner verification is incomplete."}
	verified := true
	for path, revision := range plan.protected {
		data, readErr := runtimeMutationOwnerBytes(path, true)
		verified = verified && readErr == nil && runtimeMutationRevision(data) == revision
		zeroBytes(data)
	}
	if verified {
		owner.Disposition, owner.Verified, owner.Evidence = "retained", true, "Acknowledged Core configuration, credential and registration owner bytes remain unchanged."
	}
	var receiptErr error
	result.ProviderChangeResult, receiptErr = application.RecordProviderImpactScopeResult(plan.Scope, owner, nil)
	if err != nil || !verified || receiptErr != nil || !result.Runtime.Verified {
		return result, &machine.Error{Code: machine.ErrorConflict, CauseCode: "control_plane_stop_partial", Message: "Core stop is incomplete; inspect the actual services and retained recovery ownership before retrying.", Details: result, Cause: errors.Join(err, receiptErr)}
	}
	result.State = "stopped"
	return result, nil
}
