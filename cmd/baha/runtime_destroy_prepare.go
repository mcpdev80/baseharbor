package main

import (
	"context"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

type fullDestroyInventory struct {
	targets           []deployment.ResolvedTarget
	deployments       []deployment.DeploymentRecord
	discoveryResults  []fullDestroyResult
	deploymentResults []fullDestroyResult
	plans             []targetDestroyInventory
	preserved         []fullDestroyResult
	blockedTargets    map[string]bool
}

func prepareFullDestroyInventory(parent context.Context) (fullDestroyInventory, error) {
	targets, discoveryResults := discoverFullDestroyTargets()
	deployments, deploymentResults := discoverFullDestroyDeployments()
	blockedTargets := map[string]bool{}
	for _, record := range deployments {
		if err := authorizeMCPOperation(parent, "installation.destroy", record.Identity.Target, record.Identity.Environment, record.Identity.ApplicationID, record.Source.Manifest); err != nil {
			return fullDestroyInventory{}, err
		}
	}

	plans, err := collectFullDestroyInventory(parent, targets)
	if err != nil {
		return fullDestroyInventory{}, err
	}
	for _, plan := range plans {
		if plan.InventoryError != "" {
			blockedTargets[plan.Target.Name] = true
		}
	}
	preserved := []fullDestroyResult{}
	for _, target := range targets {
		item, err := preservedTargetRecovery(parent, target)
		if err != nil {
			return fullDestroyInventory{}, fmt.Errorf("inventory external recovery material: %w", err)
		}
		if item.Status != "" {
			preserved = append(preserved, item)
		}
	}

	recordDestroyPlan(parent, plans, preserved)
	return fullDestroyInventory{
		targets: targets, deployments: deployments,
		discoveryResults: discoveryResults, deploymentResults: deploymentResults,
		plans: plans, preserved: preserved, blockedTargets: blockedTargets,
	}, nil
}
