package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"os"
)

type publicControlPlaneCheck struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}
type controlPlaneReport struct {
	Installation          *coreinstallation.State   `json:"installation,omitempty"`
	Target                string                    `json:"target"`
	State                 string                    `json:"state"`
	Ready                 bool                      `json:"ready"`
	Running               []string                  `json:"running,omitempty"`
	Checks                []publicControlPlaneCheck `json:"checks,omitempty"`
	AvailabilityDetail    string                    `json:"availability_detail,omitempty"`
	AvailabilitySatisfied bool                      `json:"availability_satisfied"`
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
	availability := evaluateControlPlaneAvailability(running, checks, files.HA)
	result.AvailabilitySatisfied = availability.Satisfied
	result.AvailabilityDetail = availability.Detail()
	if !result.Ready {
		result.State = "degraded"
	}
	return result, nil
}

type controlPlaneDoctorReport struct {
	Ready  bool                      `json:"ready"`
	Checks []publicControlPlaneCheck `json:"checks"`
}

func inspectControlPlaneDoctor(ctx context.Context) (controlPlaneDoctorReport, error) {
	if err := authorizeCurrentMCPContext(ctx, "control-plane.doctor", "", "", ""); err != nil {
		return controlPlaneDoctorReport{}, err
	}
	checks := collectControlPlaneDoctorChecks(ctx)
	result := controlPlaneDoctorReport{Ready: true, Checks: []publicControlPlaneCheck{}}
	for _, check := range checks {
		result.Checks = append(result.Checks, publicControlPlaneCheck{Name: check.Name, Ready: check.OK})
		result.Ready = result.Ready && check.OK
	}
	return result, nil
}
func collectControlPlaneDoctorChecks(ctx context.Context) []health.Check {
	// health.Doctor includes legacy default-local runtime probes. Drop those
	// and use only the effective Target's owned runtime for provider readiness.
	// This prevents a healthy named Core from being marked FAILED by empty local.
	host := health.Doctor()
	checks := make([]health.Check, 0, len(host)+3)
	for _, check := range host {
		if check.Name != "postgres" && check.Name != "openbao" && check.Name != "runtime-config" {
			checks = append(checks, check)
		}
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err == nil {
		checks = append(checks, health.RuntimeChecksForFiles(files)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		checks = append(checks, health.Check{Name: "runtime-config", OK: false, Message: "selected Target runtime state is unreadable"})
	}
	return appendControlPlaneAvailabilityDoctor(ctx, checks)
}
func requireControlPlaneReady(result controlPlaneReport) error {
	if result.State != "running" || !result.Ready {
		return machine.NewError(machine.ErrorVerificationFailed, "control plane is not ready", "Inspect control-plane.status and control-plane.doctor.", false)
	}
	return nil
}
