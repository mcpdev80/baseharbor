package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func hasRemoteApplicationTarget(resolved resolvedApplication) bool {
	return resolved.Target.AccessProvider != "" && resolved.Target.AccessProvider != "local"
}

// Local file absence cannot establish remote absence. Read the protected
// deployment binding and fresh, independently owned Node inventory instead.
// A provider observation does not qualify the complete Application lifecycle.
func collectRemoteApplicationStatus(ctx context.Context, resolved resolvedApplication) (application.StatusResult, error) {
	runtime, err := remoteApplicationProjectRuntime(ctx, resolved)
	if err != nil {
		return application.StatusResult{}, err
	}
	record, err := retainedRemoteProject(resolved)
	if err != nil {
		return application.StatusResult{}, err
	}
	if record == nil {
		return application.StatusResult{}, machine.NewError(machine.ErrorRuntimeUnavailable,
			"Remote application has no protected publication binding.",
			"Inspect this installation's deployment state before retrying; local file absence cannot establish remote absence.", false)
	}
	observed, err := runtime.ObserveProject(ctx, record.BundleID)
	if err != nil {
		return application.StatusResult{}, err
	}
	return remoteApplicationStatusObservation(resolved, record.BundleID, observed), nil
}

func remoteApplicationStatusObservation(resolved resolvedApplication, project string, observed []targetsession.ProjectService) application.StatusResult {
	m := resolved.Manifest
	result := application.StatusResult{ContractVersion: "v1", Target: resolved.Target.Name,
		Application: m.Name, Environment: m.Environment, Project: project,
		State: "incomplete", Checks: []application.StatusCheck{}}
	if resolved.FromRepository {
		result.Manifest = resolved.ManifestPath
	}
	if len(observed) == 0 {
		result.AddObservation("remote-provider-project", "unverified", "no owned provider containers observed; complete application absence is unverified")
	}
	for _, service := range observed {
		state, detail := "failed", "owned service is not running"
		if service.Running && strings.EqualFold(service.State, "running") {
			result.State = "running"
			switch strings.ToLower(strings.TrimSpace(service.Health)) {
			case "healthy":
				state, detail = "ready", "owned service native health verified"
			case "":
				state, detail = "unverified", "owned service is running without a positive health observation"
			default:
				detail = "owned service native health is not ready"
			}
		}
		result.AddObservation("remote-provider/"+service.Service, state, detail)
	}
	// Keep the existing readiness contract: running providers alone cannot
	// verify repository workloads, protocol probes, secrets or exposure.
	result.AddObservation("application-lifecycle", "unverified", "complete remote application readiness requires workload, protocol, secret and exposure verification")
	return result
}
