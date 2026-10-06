// Package machinereadmodels derives consumer artifacts from actual Core result
// types. Synthetic examples are contracts, never deployment or release evidence.
package machinereadmodels

import (
	"encoding/json"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

const SchemaID = "https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/machine/v1/read-models.schema.json"

type Example struct {
	Operation string `json:"operation"`
	Value     any    `json:"value"`
}

func Examples() []Example {
	return []Example{
		{"app.list", machine.ApplicationListResult{ContractVersion: "v1", Deployments: []machine.DeploymentSummary{{ContractVersion: "v1", DeploymentID: "synthetic-deployment", ApplicationID: "synthetic-application", Target: "synthetic-target", Application: "synthetic-demo", Environment: "prod", RuntimeProvider: "podman", State: "running", Ready: true, SourceKind: "repository", SourceAvailable: true}}}},
		{"app.list", machine.ApplicationListResult{ContractVersion: "v1", Deployments: []machine.DeploymentSummary{}, Warnings: []string{"Synthetic partial source observation"}}},
		{"target.list", machine.TargetListResult{ContractVersion: "v1", Targets: []machine.TargetSummary{{ContractVersion: "v1", Name: "synthetic-target", RuntimeProvider: "podman", AccessReference: "synthetic-node", Scope: "synthetic-scope", Default: true, Effective: true}}}},
		{"workspace.list", machine.WorkspaceListResult{ContractVersion: "v1", Workspaces: []machine.WorkspaceSummary{{ContractVersion: "v1", Application: "synthetic-demo", Manifest: "/synthetic/baseharbor.yaml", SourceCount: 1, Sources: map[string]string{"app": "/synthetic/app"}}}}},
		{"runtime.list", []runtimeexplorer.Resource{{ContractVersion: runtimeexplorer.ContractVersion, Ref: runtimeexplorer.ResourceRef{Provider: "podman", Target: "synthetic-target", Kind: runtimeexplorer.KindContainer, ResourceID: "synthetic-container"}, DisplayName: "synthetic-api", Ownership: runtimeexplorer.OwnershipManaged, Relationship: runtimeexplorer.Relationship{ApplicationID: "synthetic-application", DeploymentID: "synthetic-deployment", Environment: "prod"}, State: runtimeexplorer.ResourceState{Observed: "running"}}}},
		{"runtime.list", []runtimeexplorer.Resource(nil)},
	}
}

func Golden() ([]byte, error) {
	return json.MarshalIndent(struct {
		Schema    string    `json:"schema"`
		Synthetic bool      `json:"synthetic"`
		Records   []Example `json:"records"`
	}{"baseharbor.machine-read-model-fixtures/v1", true, Examples()}, "", "  ")
}

func Schema() ([]byte, error) {
	types := map[string]reflect.Type{
		"app.list":       reflect.TypeFor[machine.ApplicationListResult](),
		"target.list":    reflect.TypeFor[machine.TargetListResult](),
		"workspace.list": reflect.TypeFor[machine.WorkspaceListResult](),
		"runtime.list":   reflect.TypeFor[[]runtimeexplorer.Resource](),
	}
	definitions := map[string]any{}
	for operation, kind := range types {
		inferred, err := jsonschema.ForType(kind, nil)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(inferred)
		if err != nil {
			return nil, err
		}
		var definition map[string]any
		if err := json.Unmarshal(raw, &definition); err != nil {
			return nil, err
		}
		if operation == "runtime.list" {
			definition = map[string]any{"anyOf": []any{definition, map[string]any{"type": "null"}}}
		}
		definitions[operation] = definition
	}
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": SchemaID, "description": "Core result shapes derived from the semantic Go types. Native observations do not imply health or authorization.", "$defs": definitions}, "", "  ")
}
