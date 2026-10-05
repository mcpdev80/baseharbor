package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func (e *bahaMachineExecutor) executeHTTPPlatformMutation(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	switch operationID {
	case "workspace.update":
		return executeHTTPWorkspaceUpdate(ctx, operationContext, raw, report)
	case "app.new":
		return executeHTTPAppNew(ctx, operationContext, raw, report)
	case "provider.add", "provider.remove":
		return executeHTTPProviderMutation(ctx, operationID, raw, report)
	case "organization.set", "organization.update":
		return executeHTTPOrganizationMutation(ctx, operationID, operationContext, raw, report)
	case "runtime.operate":
		return executeHTTPRuntimeOperation(ctx, operationContext, raw, report)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported platform mutation.", "Use machine discovery.", false)
	}
}

func executeHTTPWorkspaceUpdate(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineWorkspaceUpdateInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	manifest, err := bindHTTPSelector("workspace", operationContext.Workspace, input.Manifest)
	if err != nil {
		return nil, err
	}
	manifestPath, manifestModel, err := resolveWorkspaceManifest(manifest)
	if err != nil {
		return nil, err
	}
	model, _, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return nil, err
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifestModel.Name)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "workspace", "Updating mapped repositories safely.", 25)
	result, err := development.UpdateWorkspaceGit(ctx, model, mapping, input.Check)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "workspace", "Workspace update completed.", 100)
	return result, nil
}

func executeHTTPAppNew(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineAppNewInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	var err error
	input.Name, err = bindHTTPSelector("application", operationContext.Application, input.Name)
	if err != nil {
		return nil, err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Application name is required.", "Provide context.application or input.name.", false)
	}

	root, err := resolveHTTPNewApplicationRoot(name, input)
	if err != nil {
		return nil, err
	}
	capabilities, err := developmentCapabilityKinds(input.Capabilities)
	if err != nil {
		return nil, err
	}
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		return nil, err
	}
	adapterID, profile, err := resolveHTTPDevelopmentSelection(input, registry)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "generate", "Generating ecosystem-native application files.", 35)
	result, err := development.CreateApplication(root, development.NewApplicationRequest{
		Name:               name,
		Environment:        strings.TrimSpace(input.Environment),
		Adapter:            adapterID,
		Profile:            profile,
		Capabilities:       capabilities,
		Secrets:            append([]string(nil), input.Secrets...),
		EmitBackstage:      input.EmitBackstage,
		BackstageOwner:     input.BackstageOwner,
		BackstageLifecycle: input.BackstageLifecycle,
	}, registry)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "validate", "Application generation validated.", 100)
	return struct {
		ContractVersion string                      `json:"contract_version"`
		Application     string                      `json:"application"`
		Environment     string                      `json:"environment"`
		Profile         development.StackProfile    `json:"profile"`
		DevelopmentPlan development.DevelopmentPlan `json:"development_plan"`
		Files           []string                    `json:"files"`
		Validation      development.Validation      `json:"validation"`
	}{
		ContractVersion: machine.ContractVersion,
		Application:     result.Manifest.Name,
		Environment:     result.Manifest.Environment,
		Profile:         result.Profile,
		DevelopmentPlan: result.Plan,
		Files:           result.FilePaths,
		Validation:      result.Validation,
	}, nil
}

func resolveHTTPNewApplicationRoot(name string, input machineAppNewInput) (string, error) {
	if strings.TrimSpace(input.Path) != "" {
		if strings.TrimSpace(input.Directory) != "" {
			return "", machine.NewError(machine.ErrorValidationFailed, "path and directory cannot be combined.", "Use directory for new clients.", false)
		}
		root, err := expandUserPath(input.Path)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(root) {
			return root, nil
		}
		return filepath.Abs(root)
	}
	return resolveNewApplicationRoot(name, input.Directory)
}

func resolveHTTPDevelopmentSelection(input machineAppNewInput, registry development.Registry) (string, *development.StackProfile, error) {
	stackProfile := strings.TrimSpace(input.StackProfile)
	if stackProfile == "" && strings.TrimSpace(input.Stack) == "" {
		var err error
		stackProfile, err = organizationDefaultStack(input.Environment)
		if err != nil {
			return "", nil, err
		}
	}
	if stackProfile != "" {
		if strings.TrimSpace(input.Stack) != "" {
			return "", nil, machine.NewError(machine.ErrorValidationFailed, "stack and stack_profile cannot be combined.", "Select one development stack source.", false)
		}
		catalog, err := effectiveDevelopmentProfileCatalog(".", registry)
		if err != nil {
			return "", nil, err
		}
		resolved, err := development.ResolveStackProfile(stackProfile, development.ProfileMap(catalog))
		if err != nil {
			return "", nil, err
		}
		return "", &resolved.Profile, nil
	}
	adapterID, err := developmentAdapterID(input.Stack)
	return adapterID, nil, err
}

func executeHTTPProviderMutation(
	ctx context.Context,
	operationID string,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	if operationID == "provider.add" {
		var input machineProviderAddInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		reg, err := providerExternalRegistration(providerExternalArgs{
			ID: input.ID, ProviderID: input.ProviderID, ProviderVersion: input.ProviderVersion,
			ProviderProtocol: input.ProviderProtocol, Kind: input.Kind,
			Capabilities: append([]string(nil), input.Capabilities...), Endpoint: input.Endpoint,
			CredentialRef: input.CredentialRef, TrustMode: input.TrustMode, CAReference: input.CAReference,
			ClientCertificate: input.ClientCertificate, ClientKey: input.ClientKey, Directory: input.CertificateDir,
		})
		if err != nil {
			return nil, err
		}
		reportHTTPProgress(report, "provider", "Registering external provider reference.", 50)
		if err := application.RegisterExternalProvider(reg); err != nil {
			return nil, err
		}
		reportHTTPProgress(report, "provider", "Provider registration completed.", 100)
		return reg.Public(), nil
	}

	var input machineProviderRemoveInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := applicationlifecycle.RequireApproval("provider.remove", input.Approval); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(input.ID)
	item, err := application.InspectExternalProvider(id)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "provider", "Removing BaseHarbor provider registration.", 50)
	if err := application.RemoveExternalProvider(id); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "provider", "Provider registration removed.", 100)
	return struct {
		ID             string `json:"id"`
		Removed        bool   `json:"removed"`
		ForeignMutated bool   `json:"foreign_mutated"`
	}{ID: item.ID, Removed: true, ForeignMutated: false}, nil
}

func executeHTTPOrganizationMutation(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	if operationID == "organization.set" {
		var input machineOrganizationSetInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		environment, err := bindHTTPSelector("environment", operationContext.Environment, input.Environment)
		if err != nil {
			return nil, err
		}
		input.Environment = environment
		source := orgconfig.Source{
			Kind:      orgconfig.SourceKind(strings.ToLower(strings.TrimSpace(input.Source))),
			Location:  strings.TrimSpace(input.Location),
			Requested: strings.TrimSpace(input.Requested),
		}
		reportHTTPProgress(report, "organization", "Resolving organization source.", 40)
		state, err := orgconfig.Activate(ctx, source)
		if err != nil {
			return nil, err
		}
		effective, err := orgconfig.ResolveEffective(state, input.Environment)
		if err != nil {
			return nil, err
		}
		reportHTTPProgress(report, "organization", "Organization source activated.", 100)
		return organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
	}

	var input machineOrganizationUpdateInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	environment, err := bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	input.Environment = environment
	if err := applicationlifecycle.RequireApproval("organization.update", input.Approval); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "organization", "Refreshing organization source.", 40)
	state, err := orgconfig.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	effective, err := orgconfig.ResolveEffective(state, input.Environment)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "organization", "Organization source updated.", 100)
	return organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
}

func reportHTTPProgress(report machinehttp.ProgressReporter, stage, message string, percent float64) {
	if report != nil {
		report(machine.OperationProgress{Stage: stage, Message: message, Percent: percent})
	}
}
