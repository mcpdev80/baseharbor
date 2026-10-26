package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func hasRemoteApplicationTarget(resolved resolvedApplication) bool {
	return resolved.Target.AccessProvider != "" && resolved.Target.AccessProvider != "local"
}

func collectRemoteApplicationStatus(ctx context.Context, resolved resolvedApplication) (application.StatusResult, error) {
	result := application.StatusResult{ContractVersion: "v1", Target: resolved.Target.Name, Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Manifest: resolved.ManifestPath, State: "incomplete", Checks: []application.StatusCheck{}}
	runtime, manifest, err := restoreRemoteApplication(ctx, resolved)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, application.ErrRuntimeNotApplied) {
		return result, machine.NewError(machine.ErrorRuntimeUnavailable, "Remote application has no retained Application publication.", "Inspect protected deployment state; local file absence cannot establish remote absence.", false)
	}
	if err != nil {
		return result, err
	}
	result.Project, result.State, result.Ready = runtime.Record().BundleID, "running", true
	err = runtime.VerifyApplication(ctx, manifest)
	result.AddCheck("remote Application readiness", err == nil, "owned service state and authenticated backend protocol readiness")
	if err != nil {
		result.State = "degraded"
	}
	return result, nil
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
