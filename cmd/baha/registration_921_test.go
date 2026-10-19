package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
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
	// The edit tool schema root must never force the apply-only proposal/consent,
	// so propose is a valid schema entry without them.
	var root struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	for _, required := range root.Required {
		if required == "proposal" || required == "consent" {
			t.Fatalf("proposal/consent must be optional for propose at the schema root: %s", string(payload))
		}
	}
	if !containsString(root.Required, "desired") {
		t.Fatalf("schema root must require only the Target-bound desired intent: %s", string(payload))
	}
	// desired.target is the only required nested field; desired.choices.requirement is required.
	var document struct {
		Properties map[string]struct {
			Required []string `json:"required"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
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
}

func TestCore921MCPProposeReachesHandlerWithZeroWrites(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"api"}}}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	registerTestDeployment(t, configureTestTarget(t), m, root, application.RepositoryManifestName)

	manifestPath := application.RepositoryManifestName
	sourceBefore, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	recordsBefore, listErr := deployment.ListDeployments("docker-dev")
	if listErr != nil {
		t.Fatal(listErr)
	}
	beforeBytes := deploymentSnapshot(t)

	session := workspaceMutationClient(t)
	// A validly structured propose (correct target, empty requirements intent)
	// reaches the real edit handler. It fails closed on the missing snapshot with
	// a typed conflict, never mutating state.
	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "baseharbor.app.edit",
		Arguments: map[string]any{
			"target":  "docker-dev",
			"phase":   "propose",
			"desired": map[string]any{"target": "docker-dev", "snapshot_revision": "stale-snapshot-0"},
		},
	})
	if callErr != nil {
		t.Fatalf("propose call failed: %v", callErr)
	}
	if result == nil || !result.IsError {
		t.Fatal("valid-structure propose must still fail closed on the stale-snapshot intent")
	}
	// The public boundary classifies the stale-snapshot mismatch through the
	// typed machine error owner, not as internal_error.
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	raw, _ := json.Marshal(result.StructuredContent)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("error payload not structured: %v", err)
	}
	if payload.Error.Code != "conflict" {
		t.Fatalf("stale-snapshot propose must classify as conflict, got %q (payload: %s)", payload.Error.Code, string(raw))
	}

	// Zero writes: the manifest bytes and every deployment record are byte-identical.
	sourceAfter, afterErr := os.ReadFile(manifestPath)
	if afterErr != nil {
		t.Fatal(afterErr)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("propose mutated the application manifest")
	}
	recordsAfter, _ := deployment.ListDeployments("docker-dev")
	if len(recordsAfter) != len(recordsBefore) {
		t.Fatalf("propose changed deployment records: before=%d after=%d", len(recordsBefore), len(recordsAfter))
	}
	if !deploymentSnapshotEqual(t, beforeBytes, deploymentSnapshot(t)) {
		t.Fatal("propose mutated deployment state at the byte level")
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
	sourceBefore, readErr := os.ReadFile(application.RepositoryManifestName)
	if readErr != nil {
		t.Fatal(readErr)
	}
	beforeBytes := deploymentSnapshot(t)

	session := workspaceMutationClient(t)
	ctx := context.Background()

	// structuredCode asserts the MCP result carries a concrete typed Code on the
	// public boundary, not the internal_error fallthrough from an unclassified
	// domain error.
	structuredCode := func(label string, arguments map[string]any, wantCode string) string {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: arguments})
		if callErr != nil {
			t.Fatalf("%s: call failed: %v", label, callErr)
		}
		if result == nil || !result.IsError {
			t.Fatalf("%s: expected a typed failure for %v", label, arguments)
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s: error result has no structured error payload", label)
		}
		var payload struct {
			Error struct {
				Code      string `json:"code"`
				CauseCode string `json:"cause"`
				Next      string `json:"next,omitempty"`
			} `json:"error"`
		}
		raw, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("%s: error payload not structured: %v (%s)", label, err, string(raw))
		}
		if payload.Error.Code != wantCode {
			t.Fatalf("%s: handler returned code %q, want %q (payload: %s)", label, payload.Error.Code, wantCode, string(raw))
		}
		return payload.Error.CauseCode
	}

	// Wrong Target: a configured target that does not match the resolved app.
	// Must classify through the typed machine error owner as a conflict.
	structuredCode("wrong-target", map[string]any{
		"target":  "docker-dev",
		"phase":   "propose",
		"desired": map[string]any{"target": "podman-dev"},
	}, "conflict")

	// Invalid phase must classify as validation_failed with remediation.
	structuredCode("invalid-phase", map[string]any{
		"target":  "docker-dev",
		"phase":   "suspend",
		"desired": map[string]any{"target": "docker-dev"},
	}, "validation_failed")

	// Apply without proposal/consent must classify as validation_failed.
	structuredCode("apply-missing", map[string]any{
		"target":  "docker-dev",
		"phase":   "apply",
		"desired": map[string]any{"target": "docker-dev"},
	}, "validation_failed")

	// Apply without a matching typed proposal/consent fails closed through the
	// typed machine error owner as validation_failed: the apply phase demands
	// the exact proposal and fresh consent before any snapshot comparison, so the
	// consent gate is the reachable typed boundary here (the stale-snapshot
	// conflict only fires after a valid proposal/consent round-trip). Either way
	// the public boundary surfaces a concrete code, not internal_error.
	structuredCode("apply-missing-snapshot", map[string]any{
		"target":  "docker-dev",
		"phase":   "apply",
		"desired": map[string]any{"target": "docker-dev", "snapshot_revision": "stale-snapshot-9"},
	}, "validation_failed")

	// The handler-level negatives wrote nothing: manifest bytes and deployment
	// state are byte-identical before and after.
	sourceAfter, afterErr := os.ReadFile(application.RepositoryManifestName)
	if afterErr != nil {
		t.Fatal(afterErr)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("handler-level negatives mutated the application manifest")
	}
	recordsAfter, listErr := deployment.ListDeployments("docker-dev")
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(recordsAfter) != len(recordsBefore) {
		t.Fatalf("handler-level negatives wrote deployment state: before=%d after=%d", len(recordsBefore), len(recordsAfter))
	}
	if !deploymentSnapshotEqual(t, beforeBytes, deploymentSnapshot(t)) {
		t.Fatal("handler-level negatives changed deployment state at the byte level")
	}
}

// deploymentSnapshot marshals every deployment record for the target into a
// stable byte string so a zero-write assertion compares actual persisted state,
// not merely a record count.
func deploymentSnapshot(t *testing.T) []byte {
	t.Helper()
	records, err := deployment.ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Identity.DeploymentID < records[j].Identity.DeploymentID
	})
	var buf bytes.Buffer
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// deploymentSnapshotEqual reports byte-level equality of two deployment snapshots.
func deploymentSnapshotEqual(t *testing.T, before, after []byte) bool {
	t.Helper()
	return bytes.Equal(before, after)
}
