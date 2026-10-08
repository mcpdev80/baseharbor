package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"os"
	"strings"
)

type machineTargetCreateInput struct {
	Name            string `json:"name"`
	RuntimeProvider string `json:"runtime_provider"`
	AccessProvider  string `json:"access_provider,omitempty"`
	Access          string `json:"access"`
	Reference       string `json:"reference"`
	Scope           string `json:"scope,omitempty"`
	Default         bool   `json:"default,omitempty"`
}
type targetMutationResult struct {
	ContractVersion string `json:"contract_version"`
	Name            string `json:"name"`
	Created         bool   `json:"created,omitempty"`
	Deleted         bool   `json:"deleted,omitempty"`
}

func createTargetDefinition(ctx context.Context, input machineTargetCreateInput) (targetMutationResult, error) {
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
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return targetMutationResult{}, err
	}
	if _, exists := cfg.Targets[name]; exists {
		return targetMutationResult{}, fmt.Errorf("target %q already exists", name)
	}
	if existing, exists := cfg.Access[accessName]; exists {
		if existing.Provider != accessProvider || existing.Reference != reference {
			return targetMutationResult{}, fmt.Errorf("access %q already exists with different provider/reference", accessName)
		}
	} else {
		cfg.Access[accessName] = deployment.AccessDefinition{Provider: accessProvider, Reference: reference}
	}
	cfg.Targets[name] = deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: runtimeProvider},
		Access:  deployment.TargetAccess{Reference: accessName},
		Scope:   scope,
	}
	if makeDefault {
		cfg.DefaultTarget = name
	}
	if err := cfg.Save(); err != nil {
		return targetMutationResult{}, err
	}
	return targetMutationResult{ContractVersion: machine.ContractVersion, Name: name, Created: true}, nil
}
func deleteTargetDefinition(ctx context.Context, name string) (targetMutationResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "target.delete", "", "", ""); err != nil {
		return targetMutationResult{}, err
	}
	if err := deployment.ValidateTargetName(name); err != nil {
		return targetMutationResult{}, err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return targetMutationResult{}, err
	}
	if _, ok := cfg.Targets[name]; !ok {
		return targetMutationResult{}, fmt.Errorf("target %q is not configured", name)
	}
	deployments, err := deployment.ListDeployments(name)
	if err != nil {
		return targetMutationResult{}, err
	}
	if len(deployments) != 0 {
		return targetMutationResult{}, fmt.Errorf("target %q still owns %d deployment(s); destroy them before deleting the target", name, len(deployments))
	}
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		return targetMutationResult{}, err
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		return targetMutationResult{}, fmt.Errorf("target %q still owns runtime state under %s; destroy or detach owned state before deleting the target", name, root)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return targetMutationResult{}, err
	}
	delete(cfg.Targets, name)
	if cfg.DefaultTarget == name {
		cfg.DefaultTarget = ""
	}
	if err := cfg.Save(); err != nil {
		return targetMutationResult{}, err
	}
	// A deleted Target must not remain activated for future CLI processes.
	active, err := readPersistedTarget()
	if err != nil {
		return targetMutationResult{}, fmt.Errorf("target deleted, but active selection could not be inspected: %w", err)
	}
	if active == name {
		if err := clearPersistedTarget(); err != nil {
			return targetMutationResult{}, fmt.Errorf("target deleted, but active selection could not be cleared: %w", err)
		}
	}
	if err := os.Remove(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return targetMutationResult{}, fmt.Errorf("remove empty target state directory: %w", err)
	}
	return targetMutationResult{ContractVersion: machine.ContractVersion, Name: name, Deleted: true}, nil
}
