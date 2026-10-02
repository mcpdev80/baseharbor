package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func (e *bahaMachineExecutor) executeHTTPRead(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	switch operationID {
	case "workspace.list":
		return collectMachineWorkspaceList()
	case "target":
		return executeHTTPTarget(ctx, operationContext, raw)
	case "target.list":
		return collectMachineTargetList(ctx)
	case "app.list":
		return executeHTTPApplicationList(ctx, operationContext, raw)
	case "inspect":
		return executeHTTPInspect(ctx, operationContext, raw)
	case "workspace.resolve", "workspace.status":
		return executeHTTPWorkspaceRead(ctx, operationID, operationContext, raw)
	case "runtime.capabilities", "runtime.list", "runtime.inspect":
		return executeHTTPRuntimeExplorerRead(ctx, operationID, operationContext, raw)
	case "plan", "status", "doctor", "observe", "evidence":
		return e.executeHTTPApplicationRead(ctx, operationID, operationContext, raw)
	case "provider.list", "provider.inspect", "provider.verify":
		return executeHTTPProviderRead(ctx, operationID, raw)
	case "organization.inspect", "organization.check":
		return executeHTTPOrganizationRead(ctx, operationID, operationContext, raw)
	case "policy.check", "policy.explain":
		return e.executeHTTPPolicyRead(ctx, operationID, operationContext, raw)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported read operation.", "Use machine discovery.", false)
	}
}

func executeHTTPApplicationList(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineApplicationListInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	return collectMachineApplicationList(withTargetOverride(ctx, input.Target), input)
}

func executeHTTPTarget(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineTargetInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	return collectTargetInspection(withTargetOverride(ctx, target))
}

func executeHTTPInspect(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineInspectInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(input.Path)
	if operationContext.Workspace != "" {
		var err error
		path, err = bindHTTPSelector("workspace", operationContext.Workspace, path)
		if err != nil {
			return nil, err
		}
	}
	if path == "" {
		path = "."
	}
	return inspectRepositorySource(ctx, path)
}

func executeHTTPWorkspaceRead(ctx context.Context, operationID string, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var manifestInput struct {
		Manifest string `json:"manifest,omitempty"`
		Fetch    bool   `json:"fetch,omitempty"`
	}
	if err := decodeHTTPInput(raw, &manifestInput); err != nil {
		return nil, err
	}
	manifestPath, err := bindHTTPSelector("workspace", operationContext.Workspace, manifestInput.Manifest)
	if err != nil {
		return nil, err
	}
	manifestPath, manifest, err := resolveWorkspaceManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	model, _, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return nil, err
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
	if err != nil {
		return nil, err
	}
	if operationID == "workspace.resolve" {
		return development.ResolveWorkspace(manifestPath, model, mapping)
	}
	return development.InspectWorkspaceGit(ctx, model, mapping, manifestInput.Fetch)
}

func (e *bahaMachineExecutor) executeHTTPApplicationRead(ctx context.Context, operationID string, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineApplicationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationInput(operationContext, &input); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	args := machineApplicationArgs(input.Name, input.Environment)

	switch operationID {
	case "plan":
		resolved, err := resolveApplication(ctx, e.store, args, "plan")
		if err != nil {
			return nil, err
		}
		return application.BuildPlan(resolved.Manifest)
	case "status":
		return collectApplicationStatusResult(ctx, e.store, args)
	case "doctor":
		return collectApplicationDoctor(ctx, e.store, args)
	case "observe":
		status, err := collectApplicationStatusResult(ctx, e.store, args)
		if err != nil {
			return nil, err
		}
		doctor, err := collectApplicationDoctor(ctx, e.store, args)
		if err != nil {
			return nil, err
		}
		return machineObserveResult{
			ContractVersion: machine.ContractVersion,
			Target:          status.Target,
			Application:     status.Application,
			Environment:     status.Environment,
			Status:          status,
			Doctor:          doctor,
		}, nil
	case "evidence":
		return collectApplicationEvidence(ctx, e.store, machineApplicationArgs(input.Name, ""), strings.TrimSpace(input.Environment))
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported application read operation.", "Use machine discovery.", false)
	}
}

func executeHTTPProviderRead(ctx context.Context, operationID string, raw json.RawMessage) (any, error) {
	switch operationID {
	case "provider.list":
		return application.ListExternalProviders()
	case "provider.inspect", "provider.verify":
		var input machineProviderIDInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		if operationID == "provider.inspect" {
			return application.InspectExternalProvider(strings.TrimSpace(input.ID))
		}
		return application.VerifyExternalProvider(ctx, strings.TrimSpace(input.ID))
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported provider read operation.", "Use machine discovery.", false)
	}
}

func executeHTTPOrganizationRead(ctx context.Context, operationID string, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	if operationID == "organization.check" {
		status, available, err := orgconfig.Check(ctx)
		if err != nil {
			return nil, err
		}
		return organizationCheckView{ContractVersion: orgconfig.ContractVersion, Status: status, Available: available}, nil
	}
	var input machineOrganizationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	environment, err := bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	state, err := orgconfig.LoadActive()
	if err != nil {
		return nil, err
	}
	effective, err := orgconfig.ResolveEffective(state, environment)
	if err != nil {
		return nil, err
	}
	return organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
}

func (e *bahaMachineExecutor) executeHTTPPolicyRead(ctx context.Context, operationID string, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineApplicationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationInput(operationContext, &input); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	args := machineApplicationArgs(input.Name, "")
	if operationID == "policy.check" {
		return collectApplicationPolicy(ctx, e.store, args, strings.TrimSpace(input.Environment))
	}
	return explainApplicationPolicy(ctx, e.store, args, strings.TrimSpace(input.Environment))
}
