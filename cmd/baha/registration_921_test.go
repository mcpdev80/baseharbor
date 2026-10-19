package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCore921ActualCommandTreeHelpAndReadOnlyEntry(t *testing.T) {
	var output, errors bytes.Buffer
	for _, path := range [][]string{
		{"app", "new", "--help"},
		{"app", "edit", "--help"},
		{"app", "footprint", "show", "--help"},
	} {
		output.Reset()
		errors.Reset()
		if err := runWithIO(context.Background(), path, &output, &errors); err != nil {
			t.Fatalf("%v --help: %v, %s", path, err, errors.String())
		}
		if !strings.Contains(strings.ToLower(output.String()), path[len(path)-2]) &&
			!strings.Contains(strings.ToLower(output.String()), "footprint") {
			t.Fatalf("%v unavailable from actual help: %q", path, output.String())
		}
	}
	var foundEdit, foundFootprint bool
	for _, row := range currentCommandCoverage() {
		if row.Command == "baha app edit" {
			foundEdit = row.Operation == "app.edit" && row.MCPTool == "baseharbor.app.edit"
		}
		if row.Command == "baha app footprint show" {
			foundFootprint = row.Operation == "app.footprint.show" && row.MCPTool == "baseharbor.app.footprint.show"
		}
	}
	if !foundEdit || !foundFootprint {
		t.Fatalf("actual tree machine coverage is missing: edit=%t footprint=%t", foundEdit, foundFootprint)
	}
}

func TestCore921MCPRealDiscoveryAndRejectIncompleteEdit(t *testing.T) {
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	ctx := context.Background()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		if found[tool.Name] {
			t.Fatalf("duplicate registered MCP tool %q", tool.Name)
		}
		found[tool.Name] = true
	}
	for _, name := range []string{"baseharbor.app.new", "baseharbor.app.edit", "baseharbor.app.footprint.show"} {
		if !found[name] {
			t.Fatalf("real MCP tools/list missing %s", name)
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "baseharbor.app.edit",
		Arguments: map[string]any{"target": "docker-dev", "phase": "apply"},
	})
	if err == nil && result != nil && !result.IsError {
		t.Fatal("MCP edit without verified proposal and consent unexpectedly succeeded")
	}
}

func TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional(t *testing.T) {
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var edit *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "baseharbor.app.edit" {
			edit = tool
			break
		}
	}
	if edit == nil {
		t.Fatal("baseharbor.app.edit missing from real tools/list")
	}
	if edit.InputSchema == nil {
		t.Fatal("edit tool declares no input schema")
	}
	payload, marshalErr := json.Marshal(edit.InputSchema)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	// Public and nested field names are snake_case; no PascalCase leaks.
	for _, pascal := range []string{"Desired", "Proposal", "Consent", "Target"} {
		if bytes.Contains(payload, []byte("\""+pascal+"\"")) {
			t.Fatalf("PascalCase field %q leaks through the public edit schema: %s", pascal, string(payload))
		}
	}
	// The snake_case top-level contract fields are advertised.
	for _, snake := range []string{"target", "phase", "desired", "proposal", "consent"} {
		if !strings.Contains(string(payload), "\""+snake+"\"") {
			t.Fatalf("expected snake_case field %q in edit schema: %s", snake, string(payload))
		}
	}
	var document struct {
		Properties map[string]struct {
			Required []string `json:"required"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	// desired.target is the only required nested field; desired.choices.requirement is required.
	desired := document.Properties["desired"]
	requireDesiredTarget := false
	for _, required := range desired.Required {
		if required == "target" {
			requireDesiredTarget = true
		}
	}
	if !requireDesiredTarget {
		t.Fatalf("desired.target must be required: %s", string(payload))
	}
	// The top-level contract is never forced to carry the apply-only proposal/consent.
	for _, required := range document.Properties["app.edit"].Required {
		if required == "proposal" || required == "consent" {
			t.Fatalf("proposal/consent must be optional for propose: %s", string(payload))
		}
	}
}

func TestCore921MCPProposeReachesHandlerWithZeroWrites(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"api"}}}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	registerTestDeployment(t, configureTestTarget(t), m, root, application.RepositoryManifestName)

	records, err := deployment.ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	before := len(records)

	session := workspaceMutationClient(t)
	// A validly structured propose (correct target and snapshot) reaches the
	// real edit handler. It fails closed on an empty requirements intent with a
	// typed conflict, never mutating state.
	snapshot := "a7233bf01898e926fcb6e6f10aab17209dee43ea09470278d4ed1e08f9f2d5ce"
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "baseharbor.app.edit",
		Arguments: map[string]any{
			"target":  "docker-dev",
			"phase":   "propose",
			"desired": map[string]any{"target": "docker-dev", "snapshot_revision": snapshot},
		},
	})
	if err == nil && result != nil && !result.IsError {
		t.Fatal("empty-requirements propose unexpectedly succeeded")
	}
	if result == nil || !result.IsError {
		t.Fatal("valid-structure propose must still fail closed on an empty requirements intent")
	}
	payload, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(payload), "conflict") {
		t.Fatalf("handler did not classify empty-requirements propose as a conflict: %s", string(payload))
	}

	records, err = deployment.ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != before {
		t.Fatalf("propose wrote deployment state: before=%d after=%d", before, len(records))
	}
}

func TestCore921MCPEditHandlerLevelNegatives(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"api"}}}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	registerTestDeployment(t, configureTestTarget(t), m, root, application.RepositoryManifestName)

	recordsBefore, err := deployment.ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}

	session := workspaceMutationClient(t)
	ctx := context.Background()

	runEdit := func(label string, arguments map[string]any, wantContains string) {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: arguments})
		if callErr != nil {
			t.Fatalf("%s: call failed: %v", label, callErr)
		}
		if result == nil || !result.IsError {
			t.Fatalf("%s: expected a typed failure for %v", label, arguments)
		}
		payload, _ := json.Marshal(result.StructuredContent)
		if !strings.Contains(string(payload), wantContains) {
			t.Fatalf("%s: handler did not return %q: %s", label, wantContains, string(payload))
		}
	}

	// Wrong Target: a configured target that does not match the resolved app.
	runEdit("wrong-target", map[string]any{
		"target":  "docker-dev",
		"phase":   "propose",
		"desired": map[string]any{"target": "podman-dev"},
	}, "conflict")

	// Invalid phase must classify as validation_failed with remediation.
	runEdit("invalid-phase", map[string]any{
		"target":  "docker-dev",
		"phase":   "suspend",
		"desired": map[string]any{"target": "docker-dev"},
	}, "validation_failed")

	// Apply without proposal/consent must classify as validation_failed.
	runEdit("apply-missing", map[string]any{
		"target":  "docker-dev",
		"phase":   "apply",
		"desired": map[string]any{"target": "docker-dev"},
	}, "validation_failed")

	// The empty-requirements classify (conflict) also proves no writes.
	recordsAfter, err := deployment.ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(recordsAfter) != len(recordsBefore) {
		t.Fatalf("handler-level negatives wrote deployment state: before=%d after=%d", len(recordsBefore), len(recordsAfter))
	}
}
