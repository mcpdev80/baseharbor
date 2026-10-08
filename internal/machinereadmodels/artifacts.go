// Package machinereadmodels derives consumer artifacts from actual Core result
// types. Synthetic examples are contracts, never deployment or release evidence.
package machinereadmodels

import (
	"encoding/json"
	"reflect"
	"time"

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
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	exit := 0
	scope := machine.OperationContext{Environment: "prod", Target: "synthetic-target", Resource: "synthetic-container"}
	return []Example{
		{"app.list", machine.ApplicationListResult{ContractVersion: "v1", Deployments: []machine.DeploymentSummary{{ContractVersion: "v1", DeploymentID: "synthetic-deployment", ApplicationID: "synthetic-application", Target: "synthetic-target", Application: "synthetic-demo", Environment: "prod", RuntimeProvider: "podman", State: "running", Ready: true, SourceKind: "repository", SourceAvailable: true}}}},
		{"app.list", machine.ApplicationListResult{ContractVersion: "v1", Deployments: []machine.DeploymentSummary{}, Warnings: []string{"Synthetic partial source observation"}}},
		{"target.list", machine.TargetListResult{ContractVersion: "v1", Targets: []machine.TargetSummary{{ContractVersion: "v1", Name: "synthetic-target", RuntimeProvider: "podman", AccessReference: "synthetic-node", Scope: "synthetic-scope", Default: true, Effective: true}}}},
		{"workspace.list", machine.WorkspaceListResult{ContractVersion: "v1", Workspaces: []machine.WorkspaceSummary{{ContractVersion: "v1", Application: "synthetic-demo", Manifest: "/synthetic/baseharbor.yaml", SourceCount: 1, Sources: map[string]string{"app": "/synthetic/app"}}}}},
		{"runtime.list", []runtimeexplorer.Resource{{ContractVersion: runtimeexplorer.ContractVersion, Ref: runtimeexplorer.ResourceRef{Provider: "podman", Target: "synthetic-target", Kind: runtimeexplorer.KindContainer, ResourceID: "synthetic-container"}, DisplayName: "synthetic-api", Ownership: runtimeexplorer.OwnershipManaged, Relationship: runtimeexplorer.Relationship{ApplicationID: "synthetic-application", DeploymentID: "synthetic-deployment", Environment: "prod"}, State: runtimeexplorer.ResourceState{Observed: "running"}}}},
		{"runtime.list", []runtimeexplorer.Resource(nil)},
		{"runtime.capabilities", runtimeexplorer.CapabilitySet{ContractVersion: runtimeexplorer.ContractVersion, Provider: "podman", Target: "synthetic-target", Capabilities: []runtimeexplorer.Capability{runtimeexplorer.CapabilityResourceInspect, runtimeexplorer.CapabilityContainerTerminal}, ResourceKinds: []runtimeexplorer.ResourceKind{runtimeexplorer.KindContainer}}},
		{"stream.descriptor", machine.StreamDescriptor{ContractVersion: "v1", StreamID: "stream_0123456789abcdef0123456789abcdef", Kind: machine.StreamExec, Actor: machine.ActorRef{Mode: "authenticated", Issuer: "https://identity.example", Subject: "synthetic-operator"}, Context: scope, ResourceKind: "container", ResourceID: "synthetic-container", CreatedAt: now}},
		{"terminal.event", machine.TerminalEvent{ContractVersion: "v1", StreamID: "stream_0123456789abcdef0123456789abcdef", Sequence: 1, Kind: "terminal.ready", OccurredAt: now}},
		{"terminal.event", machine.TerminalEvent{ContractVersion: "v1", StreamID: "stream_0123456789abcdef0123456789abcdef", Sequence: 2, Kind: "terminal.output", OccurredAt: now, Data: []byte("synthetic output\r\n")}},
		{"terminal.event", machine.TerminalEvent{ContractVersion: "v1", StreamID: "stream_0123456789abcdef0123456789abcdef", Sequence: 3, Kind: "terminal.exit", OccurredAt: now, ExitCode: &exit}},
		{"terminal.input", machine.TerminalInput{ContractVersion: "v1", Sequence: 1, Kind: "input", Data: []byte("synthetic input\r")}},
		{"terminal.input", machine.TerminalInput{ContractVersion: "v1", Sequence: 2, Kind: "resize", Rows: 24, Columns: 80}},
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
		"app.list":             reflect.TypeFor[machine.ApplicationListResult](),
		"target.list":          reflect.TypeFor[machine.TargetListResult](),
		"workspace.list":       reflect.TypeFor[machine.WorkspaceListResult](),
		"runtime.list":         reflect.TypeFor[[]runtimeexplorer.Resource](),
		"runtime.capabilities": reflect.TypeFor[runtimeexplorer.CapabilitySet](),
		"stream.descriptor":    reflect.TypeFor[machine.StreamDescriptor](),
		"terminal.event":       reflect.TypeFor[machine.TerminalEvent](),
		"terminal.input":       reflect.TypeFor[machine.TerminalInput](),
	}
	definitions := map[string]any{}
	for operation, kind := range types {
		inferred, err := jsonschema.ForType(kind, &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[[]byte](): {Type: "string", ContentEncoding: "base64"}}})
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
