package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

type machineRuntimeTargetInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses the effective target"`
	Environment string `json:"environment,omitempty" jsonschema:"explicit environment used for authorization; defaults to dev"`
}

type machineRuntimeListInput struct {
	Target        string   `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses the effective target"`
	Environment   string   `json:"environment,omitempty" jsonschema:"environment used for authorization and resource filtering; defaults to dev"`
	Kinds         []string `json:"kinds,omitempty" jsonschema:"optional provider-neutral runtime resource kinds such as container"`
	Ownership     []string `json:"ownership,omitempty" jsonschema:"optional ownership filters: managed, external, unmanaged or platform"`
	ApplicationID string   `json:"application_id,omitempty" jsonschema:"optional stable BaseHarbor application id"`
	DeploymentID  string   `json:"deployment_id,omitempty" jsonschema:"optional stable BaseHarbor deployment id"`
	Component     string   `json:"component,omitempty" jsonschema:"optional BaseHarbor workload component"`
}

type machineRuntimeInspectInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses the effective target"`
	Environment string `json:"environment,omitempty" jsonschema:"environment used for authorization and ownership verification; defaults to dev"`
	Provider    string `json:"provider,omitempty" jsonschema:"optional runtime provider identity; defaults to the target runtime provider"`
	Kind        string `json:"kind" jsonschema:"provider-neutral runtime resource kind"`
	ResourceID  string `json:"resource_id" jsonschema:"stable runtime resource id"`
}

func runtimeExplorerForTarget(ctx context.Context, targetName string) (*runtimeexplorer.Service, string, error) {
	ctx = withTargetOverride(ctx, targetName)
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, "", err
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil, "", err
	}
	direct, ok := provider.(runtimeexplorer.DirectContainerRuntime)
	if !ok {
		return nil, "", machine.NewError(
			machine.ErrorCapabilityMissing,
			"Active Runtime Provider does not implement Runtime Explorer container primitives.",
			"Use a Runtime Provider that advertises Runtime Explorer support.",
			false,
		)
	}
	backend, err := runtimeexplorer.NewCLIContainerBackend(direct)
	if err != nil {
		return nil, "", err
	}
	explorer, err := runtimeexplorer.NewService(backend, target.Name, deploymentRuntimeOwnershipResolver{})
	if err != nil {
		return nil, "", err
	}
	return explorer, target.Name, nil
}

func collectRuntimeCapabilities(ctx context.Context, input machineRuntimeTargetInput) (runtimeexplorer.CapabilitySet, error) {
	explorer, target, err := runtimeExplorerForTarget(ctx, input.Target)
	if err != nil {
		return runtimeexplorer.CapabilitySet{}, err
	}
	return explorer.Capabilities(ctx, target)
}

func collectRuntimeResources(ctx context.Context, input machineRuntimeListInput) ([]runtimeexplorer.Resource, error) {
	explorer, target, err := runtimeExplorerForTarget(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	kinds := make([]runtimeexplorer.ResourceKind, 0, len(input.Kinds))
	for _, kind := range input.Kinds {
		if value := strings.TrimSpace(kind); value != "" {
			kinds = append(kinds, runtimeexplorer.ResourceKind(value))
		}
	}
	ownership := make([]runtimeexplorer.Ownership, 0, len(input.Ownership))
	for _, value := range input.Ownership {
		if value = strings.TrimSpace(value); value != "" {
			ownership = append(ownership, runtimeexplorer.Ownership(value))
		}
	}
	return explorer.List(ctx, runtimeexplorer.ListRequest{
		Target:        target,
		Kinds:         kinds,
		Ownership:     ownership,
		ApplicationID: strings.TrimSpace(input.ApplicationID),
		DeploymentID:  strings.TrimSpace(input.DeploymentID),
		Environment:   strings.TrimSpace(input.Environment),
		Component:     strings.TrimSpace(input.Component),
	})
}

func collectRuntimeResource(ctx context.Context, input machineRuntimeInspectInput) (runtimeexplorer.Resource, error) {
	explorer, target, err := runtimeExplorerForTarget(ctx, input.Target)
	if err != nil {
		return runtimeexplorer.Resource{}, err
	}
	capabilities, err := explorer.Capabilities(ctx, target)
	if err != nil {
		return runtimeexplorer.Resource{}, err
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = capabilities.Provider
	}
	resource, err := explorer.Inspect(ctx, runtimeexplorer.ResourceRef{
		Provider:   provider,
		Target:     target,
		Kind:       runtimeexplorer.ResourceKind(strings.TrimSpace(input.Kind)),
		ResourceID: strings.TrimSpace(input.ResourceID),
	})
	if err != nil {
		return runtimeexplorer.Resource{}, err
	}
	environment := strings.TrimSpace(input.Environment)
	if environment != "" && resource.Relationship.Environment != "" && resource.Relationship.Environment != environment {
		return runtimeexplorer.Resource{}, machine.NewError(
			machine.ErrorPolicyDenied,
			"Runtime resource is outside the authorized environment.",
			"Retry with the resource environment after obtaining the required operator authorization.",
			false,
		)
	}
	return resource, nil
}

func executeHTTPRuntimeExplorerRead(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	switch operationID {
	case "runtime.capabilities":
		var input machineRuntimeTargetInput
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
		return collectRuntimeCapabilities(ctx, input)
	case "runtime.list":
		var input machineRuntimeListInput
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
		return collectRuntimeResources(ctx, input)
	case "runtime.inspect":
		var input machineRuntimeInspectInput
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
		return collectRuntimeResource(ctx, input)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported Runtime Explorer read operation.", "Use machine discovery.", false)
	}
}
