package main

import (
	"context"
	"errors"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Progress is diagnostic metadata for a fresh approval request, never an
// acknowledgement or a persisted authority. The next invocation independently
// rebuilds the same remaining plan through normal discovery and validation.
type installationDestroyContinuation struct {
	PreviousPlanID         string   `json:"previous_plan_id"`
	CompletedDeploymentIDs []string `json:"completed_deployment_ids"`
}

func currentInstallationDestructionImpact(ctx context.Context) (runtimeDestroyImpact, error) {
	targets, discovery := discoverFullDestroyTargets()
	_, deployments := discoverFullDestroyDeployments()
	if countFullDestroyBlockers(discovery)+countFullDestroyBlockers(deployments) > 0 {
		return runtimeDestroyImpact{}, invalidProviderConsumerInventory(errors.New("remaining installation inventory is incomplete"))
	}
	plans, err := collectFullDestroyInventory(ctx, targets)
	if err != nil {
		return runtimeDestroyImpact{}, err
	}
	preserved := []fullDestroyResult{}
	for _, target := range targets {
		item, err := preservedTargetRecovery(ctx, target)
		if err != nil {
			return runtimeDestroyImpact{}, err
		}
		if item.Status != "" {
			preserved = append(preserved, item)
		}
	}
	return buildRuntimeDestroyImpact(ctx, plans, preserved, true)
}

func advanceInstallationDestruction(ctx context.Context, previous runtimeDestroyImpact, completed string, out io.Writer) (context.Context, runtimeDestroyImpact, error) {
	next, err := currentInstallationDestructionImpact(ctx)
	if err != nil {
		return ctx, next, err
	}
	next.Continuation = &installationDestroyContinuation{PreviousPlanID: previous.Scope.PlanID, CompletedDeploymentIDs: []string{completed}}
	// Earlier child completion changes the real union. Never silently refresh
	// its sibling's grant using the old parent acknowledgement.
	fresh := withProviderChangeConsent(ctx, nil)
	accepted, err := authorizeRuntimeDestroyImpact(fresh, next, true, out)
	if err != nil || !accepted {
		return ctx, next, err
	}
	current, err := currentInstallationDestructionImpact(fresh)
	if err != nil {
		return ctx, next, err
	}
	if current.Scope.PlanID != next.Scope.PlanID {
		return ctx, current, runtimeDestroyImpactError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	fresh, err = withInstallationApplicationScopes(fresh, current)
	return fresh, current, err
}
