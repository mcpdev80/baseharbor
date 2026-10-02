package main

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineApplicationListInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor target; defaults to the effective target"`
	Environment string `json:"environment,omitempty" jsonschema:"authorization environment for the listing; defaults from current repository context"`
}

type machineDeploymentSummary struct {
	ContractVersion string `json:"contract_version"`
	DeploymentID    string `json:"deployment_id"`
	ApplicationID   string `json:"application_id"`
	Target          string `json:"target"`
	Application     string `json:"application"`
	Environment     string `json:"environment"`
	RuntimeProvider string `json:"runtime_provider,omitempty"`
	State           string `json:"state,omitempty"`
	Ready           bool   `json:"ready"`
	SourceKind      string `json:"source_kind,omitempty"`
	SourceAvailable bool   `json:"source_available"`
}

type machineApplicationListResult struct {
	ContractVersion string                     `json:"contract_version"`
	Deployments     []machineDeploymentSummary `json:"deployments"`
	Warnings        []string                   `json:"warnings,omitempty"`
}

type machineWorkspaceSummary struct {
	ContractVersion string            `json:"contract_version"`
	Application     string            `json:"application"`
	Manifest        string            `json:"manifest"`
	SourceCount     int               `json:"source_count"`
	Sources         map[string]string `json:"sources,omitempty"`
}

type machineWorkspaceListResult struct {
	ContractVersion string                    `json:"contract_version"`
	Workspaces      []machineWorkspaceSummary `json:"workspaces"`
	Warnings        []string                  `json:"warnings,omitempty"`
}

type machineTargetSummary struct {
	ContractVersion string `json:"contract_version"`
	Name            string `json:"name"`
	RuntimeProvider string `json:"runtime_provider"`
	AccessReference string `json:"access_reference"`
	Scope           string `json:"scope,omitempty"`
	Implicit        bool   `json:"implicit,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Active          bool   `json:"active,omitempty"`
	Effective       bool   `json:"effective,omitempty"`
}

type machineTargetListResult struct {
	ContractVersion string                 `json:"contract_version"`
	Targets         []machineTargetSummary `json:"targets"`
}

func collectMachineApplicationList(ctx context.Context, input machineApplicationListInput) (machineApplicationListResult, error) {
	var (
		items    []deployment.DeploymentRecord
		warnings []error
		err      error
	)
	targetName := strings.TrimSpace(input.Target)
	if targetName == "" {
		target, resolveErr := effectiveTarget(ctx)
		if resolveErr != nil {
			return machineApplicationListResult{}, resolveErr
		}
		targetName = target.Name
	}
	items, warnings, err = deployment.ListDeploymentsForDisplay(targetName)
	if err != nil {
		return machineApplicationListResult{}, err
	}
	result := machineApplicationListResult{
		ContractVersion: machine.ContractVersion,
		Deployments:     make([]machineDeploymentSummary, 0, len(items)),
	}
	for _, item := range items {
		result.Deployments = append(result.Deployments, machineDeploymentSummary{
			ContractVersion: machine.ContractVersion,
			DeploymentID:    item.Identity.DeploymentID,
			ApplicationID:   item.Identity.ApplicationID,
			Target:          item.Identity.Target,
			Application:     item.Identity.Application,
			Environment:     item.Identity.Environment,
			RuntimeProvider: item.Applied.RuntimeProvider,
			State:           item.Observed.State,
			Ready:           item.Observed.Ready,
			SourceKind:      item.Source.Kind,
			SourceAvailable: deployment.SourceAvailable(item.Source),
		})
	}
	for _, warning := range warnings {
		result.Warnings = append(result.Warnings, warning.Error())
	}
	return result, nil
}

func collectMachineTargetList(ctx context.Context) (machineTargetListResult, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return machineTargetListResult{}, err
	}
	activated := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
	effective, err := effectiveTarget(ctx)
	if err != nil {
		return machineTargetListResult{}, err
	}
	names := cfg.TargetNames()
	if _, configured := cfg.Targets["local"]; !configured {
		names = append(names, "local")
		sort.Strings(names)
	}
	result := machineTargetListResult{
		ContractVersion: machine.ContractVersion,
		Targets:         make([]machineTargetSummary, 0, len(names)),
	}
	for _, name := range names {
		summary := machineTargetSummary{
			ContractVersion: machine.ContractVersion,
			Name:            name,
			Default:         name == cfg.DefaultTarget,
			Active:          name == activated,
			Effective:       name == effective.Name,
		}
		if target, configured := cfg.Targets[name]; configured {
			summary.RuntimeProvider = target.Runtime.Provider
			summary.AccessReference = target.Access.Reference
			summary.Scope = target.Scope
		} else {
			summary.RuntimeProvider = "docker"
			summary.AccessReference = "local"
			summary.Scope = "default"
			summary.Implicit = true
		}
		result.Targets = append(result.Targets, summary)
	}
	return result, nil
}

func collectMachineWorkspaceList() (machineWorkspaceListResult, error) {
	mappings, warnings, err := development.ListWorkspaceMappings()
	if err != nil {
		return machineWorkspaceListResult{}, err
	}
	result := machineWorkspaceListResult{
		ContractVersion: machine.ContractVersion,
		Workspaces:      make([]machineWorkspaceSummary, 0, len(mappings)),
	}
	for _, mapping := range mappings {
		sources := make(map[string]string, len(mapping.Sources))
		for id, root := range mapping.Sources {
			sources[id] = root
		}
		result.Workspaces = append(result.Workspaces, machineWorkspaceSummary{
			ContractVersion: machine.ContractVersion,
			Application:     mapping.Application,
			Manifest:        mapping.Manifest,
			SourceCount:     len(mapping.Sources),
			Sources:         sources,
		})
	}
	for _, warning := range warnings {
		result.Warnings = append(result.Warnings, warning.Error())
	}
	return result, nil
}
