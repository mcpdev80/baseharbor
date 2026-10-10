package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	goruntime "runtime"
	"time"
)

type publicControlPlaneCheck struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}
type controlPlaneReport struct {
	DockerEngine          *bhruntime.DockerEngineObservation `json:"docker_engine,omitempty"`
	Installation          *coreinstallation.State            `json:"installation,omitempty"`
	Target                string                             `json:"target"`
	State                 string                             `json:"state"`
	Ready                 bool                               `json:"ready"`
	Running               []string                           `json:"running,omitempty"`
	Checks                []publicControlPlaneCheck          `json:"checks,omitempty"`
	AvailabilityDetail    string                             `json:"availability_detail,omitempty"`
	AvailabilitySatisfied bool                               `json:"availability_satisfied"`
}

func inspectControlPlane(ctx context.Context) (controlPlaneReport, error) {
	if err := authorizeCurrentMCPContext(ctx, "control-plane.status", "", "", ""); err != nil {
		return controlPlaneReport{}, err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return controlPlaneReport{}, err
	}
	result := controlPlaneReport{Target: target.Name, State: "not_deployed"}
	result.DockerEngine, _ = inspectTargetDockerEngine(ctx, target)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return result, err
	}
	state, stateErr := coreinstallation.Load(root)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return result, stateErr
	}
	if stateErr == nil {
		if state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
			return result, machine.NewError(machine.ErrorOwnershipAmbiguous, "Core belongs to another installation selection.", "Select the owning installation.", false)
		}
		result.Installation = &state
		result.State = "degraded"
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return result, err
	}
	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		return result, err
	}
	result.Running = running
	result.State = "stopped"
	if len(running) == 0 {
		if result.Installation != nil {
			result.Installation.Ready = false
		}
		return result, nil
	}
	checks := health.RuntimeChecksForFiles(files)
	result.State = "running"
	result.Ready = true
	for _, check := range checks {
		result.Checks = append(result.Checks, publicControlPlaneCheck{Name: check.Name, Ready: check.OK})
		result.Ready = result.Ready && check.OK
	}
	coreReady := stateErr == nil && state.Ready && state.Spec.Target == target.Name && state.Spec.Runtime == target.RuntimeProvider
	if coreReady {
		dataDir, err := targetDataRoot(target)
		if err != nil {
			return result, err
		}
		coreReady = identityprovider.VerifyCoreIdentity(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer) == nil
	}
	result.Checks = append(result.Checks, publicControlPlaneCheck{Name: "Core Identity", Ready: coreReady})
	result.Ready = result.Ready && coreReady
	if stateErr == nil {
		state.Ready = result.Ready
		result.Installation = &state
	}
	availability, err := collectControlPlaneAvailability(ctx, checks)
	if err != nil {
		return result, err
	}
	result.AvailabilitySatisfied = availability.Satisfied
	result.AvailabilityDetail = availability.Detail()
	if !result.Ready {
		result.State = "degraded"
	}
	return result, nil
}

type controlPlaneDoctorReport struct {
	DockerEngine *bhruntime.DockerEngineObservation `json:"docker_engine,omitempty"`
	Ready        bool                               `json:"ready"`
	Checks       []publicControlPlaneCheck          `json:"checks"`
}

func inspectControlPlaneDoctor(ctx context.Context) (controlPlaneDoctorReport, error) {
	if err := authorizeCurrentMCPContext(ctx, "control-plane.doctor", "", "", ""); err != nil {
		return controlPlaneDoctorReport{}, err
	}
	checks := collectControlPlaneDoctorChecks(ctx)
	result := controlPlaneDoctorReport{Ready: true, Checks: []publicControlPlaneCheck{}}
	if target, err := effectiveTarget(ctx); err == nil {
		result.DockerEngine, _ = inspectTargetDockerEngine(ctx, target)
	}
	for _, check := range checks {
		result.Checks = append(result.Checks, publicControlPlaneCheck{Name: check.Name, Ready: check.OK})
		result.Ready = result.Ready && check.OK
	}
	result.Ready = result.Ready && !controlPlaneNotDeployed(checks)
	return result, nil
}
func controlPlaneNotDeployed(checks []health.Check) bool {
	for _, check := range checks {
		if check.Name == "core-deployment" && check.OK {
			return true
		}
	}
	return false
}
func collectControlPlaneDoctorChecks(ctx context.Context) []health.Check {
	// health.Doctor includes legacy default-local runtime probes. Drop those
	// and use only the effective Target's owned runtime for provider readiness.
	// This prevents a healthy named Core from being marked FAILED by empty local.
	checks := []health.Check{{Name: "os", OK: goruntime.GOOS == "linux" || goruntime.GOOS == "darwin" || goruntime.GOOS == "windows", Message: goruntime.GOOS + "/" + goruntime.GOARCH}}
	target, targetErr := effectiveTarget(ctx)
	if targetErr != nil {
		return append(checks, health.Check{Name: "target-selection", OK: false, Message: targetErr.Error()})
	}
	if engine, err := inspectTargetDockerEngine(ctx, target); engine != nil {
		message := fmt.Sprintf("%s (%s; daemon %s)", engine.Endpoint, engine.Mode, engine.DaemonID)
		if err != nil {
			message = err.Error()
		}
		checks = append(checks, health.Check{Name: "Docker engine", OK: err == nil && engine.Verified, Message: message})
	}
	if target.AccessProvider == "" || target.AccessProvider == "local" {
		state, stateErr := managedTrustCoreState(ctx)
		if stateErr == nil && state == "not_installed" {
			root, rootErr := targetRuntimeStateRoot(target)
			if rootErr != nil {
				stateErr = rootErr
			} else {
				entries, readErr := os.ReadDir(root)
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					stateErr = readErr
				} else {
					for _, entry := range entries {
						if entry.Name() != "docker-engine.json" {
							state = "incomplete"
							break
						}
					}
				}
			}
		}
		if stateErr != nil {
			return append(checks, health.Check{Name: "runtime-config", OK: false, Message: "selected Target runtime state is unreadable; inspect the installation before retrying"})
		}
		if state == "not_installed" {
			if provider, runtimeErr := detectRuntimeForTarget(ctx, target); runtimeErr == nil {
				resources, inventoryErr := provider.ListOwnedProjectResources(ctx, targetRuntimeProjectName(target))
				if inventoryErr != nil {
					return append(checks, health.Check{Name: "runtime-config", OK: false, Message: "Core absence could not be verified; inspect runtime ownership before retrying"})
				}
				if len(resources) > 0 {
					state = "incomplete"
				}
			}
		}
		if state == "not_installed" {
			return append(checks, health.Check{Name: "core-deployment", OK: true, Message: "NOT DEPLOYED: this Target has no materialized Core; use baha up when ready to install it"})
		}
		if state == "incomplete" {
			return append(checks, health.Check{Name: "runtime-config", OK: false, Message: "Core installation is incomplete; inspect retained installation state before retrying baha up"})
		}
	}
	if target.AccessProvider == "" || target.AccessProvider == "local" {
		_, runtimeErr := detectRuntimeForTarget(ctx, target)
		checks = append(checks, health.Check{Name: "selected-runtime", OK: runtimeErr == nil, Message: selectedRuntimeDoctorMessage(target.RuntimeProvider, runtimeErr)})
	} else {
		_, _, accessErr := runtimeExplorerForTarget(ctx, target.Name)
		checks = append(checks, health.Check{Name: "target-access", OK: accessErr == nil, Message: selectedRuntimeDoctorMessage(target.AccessProvider, accessErr)})
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err == nil {
		checks = append(checks, health.RuntimeChecksForFiles(files)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		checks = append(checks, health.Check{Name: "runtime-config", OK: false, Message: "selected Target runtime state is unreadable"})
	}
	if target.AccessProvider == "" || target.AccessProvider == "local" {
		if root, err := targetRuntimeStateRoot(target); err == nil {
			if state, err := coreinstallation.Load(root); err == nil && state.Capabilities["identity"] {
				probe, cancel := context.WithTimeout(ctx, 10*time.Second)
				dataDir, identityErr := targetDataRoot(target)
				if identityErr == nil {
					identityErr = identityprovider.VerifyCoreIdentity(probe, dataDir, target.Name, state.ID, state.IdentityIssuer)
				}
				cancel()
				message := "owned realm, database-backed authentication and discovery verified"
				if identityErr != nil {
					message = "Core Identity is unavailable; inspect its SQL trust and protected provider diagnostics"
				}
				checks = append(checks, health.Check{Name: "Core Identity", OK: identityErr == nil, Message: message})
			}
		}
	}
	return appendControlPlaneAvailabilityDoctor(ctx, checks)
}
func requireControlPlaneReady(result controlPlaneReport) error {
	if result.State != "running" || !result.Ready {
		return machine.NewError(machine.ErrorVerificationFailed, "control plane is not ready", "Inspect control-plane.status and control-plane.doctor.", false)
	}
	return nil
}

func selectedRuntimeDoctorMessage(provider string, err error) string {
	if err != nil {
		return fmt.Sprintf("%s unavailable: %v", provider, err)
	}
	return provider + " available"
}
