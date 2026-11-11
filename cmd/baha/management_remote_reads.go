package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

func currentRepositoryApplicationSelector() (string, string, bool) {
	path, err := application.FindRepositoryManifest(".")
	if err != nil {
		return "", "", false
	}
	manifest, err := application.LoadManifestFile(path)
	if err != nil || manifest.Name == "" {
		return "", "", false
	}
	return manifest.Name, manifest.Environment, true
}

// remoteMachineRead executes existing read-only Machine operations on the
// selected Core. It keeps repository planning and diagnostics on the machine
// that owns the application runtime instead of trying to open a local or
// Connector runtime on the workstation.
func remoteMachineRead(ctx context.Context, target deployment.ResolvedTarget, operation, application, environment string, input any) (json.RawMessage, error) {
	switch operation {
	case "plan", "status", "doctor", "app.preflight", "app.list", "control-plane.status", "control-plane.doctor":
	default:
		return nil, fmt.Errorf("unsupported remote read operation %q", operation)
	}
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" {
		return nil, machine.NewError(machine.ErrorPolicyDenied, "Remote application reads currently require the dev operator session.", "Select --environment dev; non-dev remote sessions require separately verified OIDC scoping.", false)
	}
	needsApplication := operation == "plan" || operation == "status" || operation == "doctor" || operation == "app.preflight"
	if needsApplication && application == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Remote application read needs an explicit application name.", "Pass the application name or run inside a repository with baseharbor.yaml.", false)
	}
	var payload []byte
	if input == nil {
		payload = []byte("{}")
	} else {
		var err error
		payload, err = json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("encode remote read request: %w", err)
		}
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorAuthenticationFailed, err, "Configure and authenticate the pinned remote Core profile for this Target.", false)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("create remote read request key: %w", err)
	}
	selectors := machine.OperationContext{Application: application, Environment: environment, Target: target.Name}
	admitted, err := core.requestMachineExecution(ctx, operation, selectors, payload, hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, readErr := core.confirmRemoteExecutionSuccess(readCtx, admitted)
		if readErr == nil {
			if !json.Valid(result) {
				return nil, errors.New("remote Core read result is not valid JSON")
			}
			return result, nil
		}
		if !errors.Is(readErr, errRemoteExecutionPending) {
			return nil, readErr
		}
		select {
		case <-readCtx.Done():
			return nil, fmt.Errorf("remote Core %s read did not finish: %w", operation, readCtx.Err())
		case <-ticker.C:
		}
	}
}

func remoteCoreTarget(target deployment.ResolvedTarget) bool {
	return target.AccessProvider == string(targetaccess.ProviderCoreRemote)
}

func remoteControlPlaneStatus(ctx context.Context, target deployment.ResolvedTarget) (controlPlaneReport, error) {
	result, err := remoteMachineRead(ctx, target, "control-plane.status", "", "dev", nil)
	if err != nil {
		return controlPlaneReport{}, err
	}
	var report controlPlaneReport
	if err := json.Unmarshal(result, &report); err != nil || report.Target != target.Name {
		return controlPlaneReport{}, errors.New("remote Core status does not match the selected Target")
	}
	return report, nil
}

func renderRemoteControlPlaneStatus(ctx context.Context, target deployment.ResolvedTarget, report controlPlaneReport, out io.Writer) error {
	renderDockerEngine(out, report.DockerEngine)
	fmt.Fprintf(out, "BaseHarbor · %s\n\nTarget\n  EFFECTIVE  %s\n  Runtime    %s\n  Access     %s (%s)\n", target.Name, target.Name, target.RuntimeProvider, target.AccessReference, target.AccessProvider)
	if target.Scope != "" {
		fmt.Fprintf(out, "  Scope      %s\n", target.Scope)
	}
	fmt.Fprintln(out, "\nControl Plane")
	switch report.State {
	case "not_deployed":
		fmt.Fprintln(out, "  NOT DEPLOYED")
		return nil
	case "stopped":
		fmt.Fprintln(out, "  STOPPED")
	default:
		if len(report.Checks) == 0 {
			fmt.Fprintf(out, "  RUNNING    %d service(s)\n", len(report.Running))
		}
		for _, check := range report.Checks {
			state := "READY"
			if !check.Ready {
				state = "FAILED"
			}
			fmt.Fprintf(out, "  %-9s %s\n", state, check.Name)
		}
		if !report.Ready {
			return errors.New("remote Core is running but not ready")
		}
		availability := "SATISFIED"
		if !report.AvailabilitySatisfied {
			availability = "UNSATISFIED"
		}
		fmt.Fprintf(out, "  %-9s availability · %s\n", availability, report.AvailabilityDetail)
	}
	result, err := remoteMachineRead(ctx, target, "app.list", "", "dev", machineApplicationListInput{Target: target.Name})
	if err != nil {
		return fmt.Errorf("list remote Core applications: %w", err)
	}
	var applications machineApplicationListResult
	if err := json.Unmarshal(result, &applications); err != nil {
		return errors.New("remote Core application list is invalid")
	}
	fmt.Fprintln(out, "\nApplications")
	fmt.Fprintf(out, "  %d registered deployment(s)\n", len(applications.Deployments))
	for _, warning := range applications.Warnings {
		fmt.Fprintf(out, "  WARN       %s\n", warning)
	}
	return nil
}

func remoteControlPlaneDoctor(ctx context.Context, target deployment.ResolvedTarget) (controlPlaneDoctorReport, error) {
	result, err := remoteMachineRead(ctx, target, "control-plane.doctor", "", "dev", nil)
	if err != nil {
		return controlPlaneDoctorReport{}, err
	}
	var report controlPlaneDoctorReport
	if err := json.Unmarshal(result, &report); err != nil {
		return controlPlaneDoctorReport{}, errors.New("remote Core doctor response is invalid")
	}
	return report, nil
}

func remoteControlPlaneRepair(ctx context.Context, target deployment.ResolvedTarget) (controlPlaneReport, error) {
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return controlPlaneReport{}, fmt.Errorf("verify remote Core connection: %w", err)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return controlPlaneReport{}, fmt.Errorf("create remote repair request key: %w", err)
	}
	selectors := machine.OperationContext{Environment: "dev", Target: target.Name}
	admitted, err := core.requestMachineExecution(ctx, "control-plane.repair", selectors, []byte("{}"), hex.EncodeToString(key))
	if err != nil {
		return controlPlaneReport{}, fmt.Errorf("request remote Core repair: %w", err)
	}
	observationCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, readErr := core.confirmRemoteExecutionSuccess(observationCtx, admitted)
		if readErr == nil {
			var report controlPlaneReport
			if err := json.Unmarshal(result, &report); err != nil || report.Target != target.Name || !report.Ready {
				return controlPlaneReport{}, errors.New("remote Core repair did not verify readiness for the selected Target")
			}
			return report, nil
		}
		if !errors.Is(readErr, errRemoteExecutionPending) {
			return controlPlaneReport{}, readErr
		}
		select {
		case <-observationCtx.Done():
			return controlPlaneReport{}, fmt.Errorf("remote Core repair execution %s is incomplete; inspect it before retrying: %w", admitted.ExecutionID, observationCtx.Err())
		case <-ticker.C:
		}
	}
}
