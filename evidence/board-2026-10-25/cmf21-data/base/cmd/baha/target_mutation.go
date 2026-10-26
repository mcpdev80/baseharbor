package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"strings"
)

type machineTargetCreateInput struct {
	RecoveryFile          string                             `json:"recovery_file,omitempty"`
	ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
	DockerEndpoint        string                             `json:"docker_endpoint,omitempty"`
	DockerContext         string                             `json:"docker_context,omitempty"`
	DockerMode            string                             `json:"docker_mode,omitempty"`
	Name                  string                             `json:"name"`
	TenantID              string                             `json:"tenant_id,omitempty"`
	RuntimeProvider       string                             `json:"runtime_provider"`
	AccessProvider        string                             `json:"access_provider,omitempty"`
	Access                string                             `json:"access"`
	Reference             string                             `json:"reference"`
	Scope                 string                             `json:"scope,omitempty"`
	Default               bool                               `json:"default,omitempty"`
}
type targetMutationResult struct {
	RecoveryFile         string                                 `json:"recovery_file,omitempty"`
	ProviderChangeResult *application.ProviderImpactScopeResult `json:"provider_change_result,omitempty"`
	RetainedMaterials    []application.MaterialOutcome          `json:"retained_materials,omitempty"`
	ContractVersion      string                                 `json:"contract_version"`
	Name                 string                                 `json:"name"`
	Created              bool                                   `json:"created,omitempty"`
	Deleted              bool                                   `json:"deleted,omitempty"`
}

func createTargetDefinition(ctx context.Context, input machineTargetCreateInput) (targetMutationResult, error) {
	if input.RecoveryFile != "" {
		if input.RuntimeProvider != "" || input.AccessProvider != "" || input.Access != "" || input.Reference != "" || input.TenantID != "" || input.Scope != "" || input.Default || input.DockerEndpoint != "" || input.DockerContext != "" || input.DockerMode != "" {
			return targetMutationResult{}, errors.New("Target recovery cannot override original runtime, access, authentication or recovery references")
		}
		if input.ProviderChangeConsent != nil {
			ctx = withProviderChangeConsent(ctx, input.ProviderChangeConsent)
		}
		return changeTargetDefinition(ctx, input.Name, input.RecoveryFile, nil)
	}
	if err := authorizeCurrentMCPContext(ctx, "target.create", "", "", ""); err != nil {
		return targetMutationResult{}, err
	}
	name := strings.TrimSpace(input.Name)
	if err := deployment.ValidateTargetName(name); err != nil {
		return targetMutationResult{}, err
	}
	runtimeProvider, accessProvider, accessName, reference, scope, makeDefault := strings.TrimSpace(input.RuntimeProvider), strings.TrimSpace(input.AccessProvider), strings.TrimSpace(input.Access), strings.TrimSpace(input.Reference), strings.TrimSpace(input.Scope), input.Default
	if runtimeProvider == "" || accessName == "" || reference == "" {
		return targetMutationResult{}, machine.Wrap(machine.ErrorValidationFailed, fmt.Errorf("runtime provider, access and reference are required"), "Provide explicit target configuration.", false)
	}
	if accessProvider == "" {
		if reference == "local" {
			accessProvider = "local"
		} else {
			return targetMutationResult{}, machine.Wrap(machine.ErrorValidationFailed, fmt.Errorf("non-local target requires an access provider"), "Select a configured target access provider.", false)
		}
	}
	cfg, expected, err := deployment.LoadConfigForMutation()
	if err != nil {
		return targetMutationResult{}, err
	}
	if _, exists := cfg.Targets[name]; exists {
		return targetMutationResult{}, machine.Wrap(machine.ErrorInternal, fmt.Errorf("target %q already exists", name), "Run baha target list to inspect existing targets, or choose another target name.", false)
	}
	if existing, exists := cfg.Access[accessName]; exists {
		if existing.Provider != accessProvider || existing.Reference != reference {
			return targetMutationResult{}, fmt.Errorf("access %q already exists with different provider/reference", accessName)
		}
	} else {
		cfg.Access[accessName] = deployment.AccessDefinition{Provider: accessProvider, Reference: reference}
	}
	cfg.Targets[name] = deployment.TargetDefinition{
		TenantID: strings.TrimSpace(input.TenantID),
		Runtime:  deployment.RuntimeDefinition{Provider: runtimeProvider, DockerEndpoint: strings.TrimSpace(input.DockerEndpoint), DockerContext: strings.TrimSpace(input.DockerContext), DockerMode: strings.TrimSpace(input.DockerMode)},
		Access:   deployment.TargetAccess{Reference: accessName},
		Scope:    scope,
	}
	if makeDefault {
		cfg.DefaultTarget = name
	}
	if err := cfg.SaveExpected(expected); err != nil {
		return targetMutationResult{}, err
	}
	return targetMutationResult{ContractVersion: machine.ContractVersion, Name: name, Created: true}, nil
}
func deleteTargetDefinition(ctx context.Context, name string) (targetMutationResult, error) {
	return changeTargetDefinition(ctx, name, "", nil)
}
