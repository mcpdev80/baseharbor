package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"os"
)

type publicControlPlaneCheck struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}
type controlPlaneReport struct {
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
		return result, nil
	}
	checks := health.RuntimeChecksForFiles(files)
	result.State = "running"
	result.Ready = true
	for _, check := range checks {
		result.Checks = append(result.Checks, publicControlPlaneCheck{Name: check.Name, Ready: check.OK})
		result.Ready = result.Ready && check.OK
	}
	availability := evaluateControlPlaneAvailability(running, checks)
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
	return appendControlPlaneAvailabilityDoctor(ctx, health.Doctor())
}
func requireControlPlaneReady(result controlPlaneReport) error {
	if result.State != "running" || !result.Ready {
		return machine.NewError(machine.ErrorVerificationFailed, "control plane is not ready", "Inspect control-plane.status and control-plane.doctor.", false)
	}
	return nil
}
