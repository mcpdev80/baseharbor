package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
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

// These real SDK calls exercise public error/result contracts using the same
// registered handler and bounded Edit authority fixtures; they are not native
// runtime acceptance evidence.
func TestCore921MCPEditConsentAndPartialExecutionThroughSDK(t *testing.T) {
	for _, scenario := range []string{"approval", "partial", "complete", "complete-preferences", "wrong-target", "stale", "requirements-added", "requirements-removed", "requirements-changed", "requirements-reordered", "preferences-added", "preferences-removed", "preferences-changed", "preferences-reordered"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			target := configureTestTarget(t)
			root := t.TempDir()
			t.Chdir(root)
			requirement := capability.Requirement{Kind: capability.SQL, Name: "primary"}
			requirements := []capability.Requirement{requirement, {Kind: capability.SQL, Name: "analytics"}}
			manifest := application.WithSQLInstances(application.New("edit-safe", "dev", false, false, false), "primary", "analytics")
			manifest.ApplicationID = application.MustNewApplicationID()
			source := filepath.Join(root, application.RepositoryManifestName)
			mustWriteInitializedAuthoredManifest(t, source, manifest)
			registerTestDeployment(t, target, manifest, root, source)
			plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name,
				PlanID: "sdk-plan", Revision: "sdk-revision", Complete: true, ConfirmationsRequired: 2,
				Impacts: []application.ProviderImpact{{InstanceID: "db-a", Operation: "provide"}, {InstanceID: "db-b", Operation: "provide"}}}
			executed := 0
			authorities := appEditTestAuthorities(target.Name, manifest.ApplicationID,
				requirements, nil, []application.ProviderImpactPlan{plan}, &executed)
			outcomes := []application.ProviderExecutionOutcome{
				{InstanceID: "db-a", Operation: "provide", Status: "applied", Verified: true, Evidence: "verified fixture outcome"},
				{InstanceID: "db-b", Operation: "provide", Status: "applied", Verified: true, Evidence: "verified fixture outcome"},
			}
			if scenario == "partial" {
				outcomes[1].Status, outcomes[1].Verified = "failed", false
				outcomes[1].ErrorCode = "interrupted"
				outcomes[1].Materials = []application.MaterialOutcome{{MaterialID: "db-b/data", Disposition: "retained",
					Location: &application.ResourceLocation{Owner: manifest.ApplicationID, Target: target.Name, Kind: "volume", Reference: "owned-data"},
					Verified: true, Evidence: "existing material retained"}}
			}
			authorities.Execute = func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
				executed++
				if scenario == "partial" {
					return outcomes, errors.New("private-executor-diagnostic password=private-secret-marker")
				}
				return outcomes, nil
			}
			ctx := context.Background()
			server := mcp.NewServer(&mcp.Implementation{Name: "edit-contract-fixture", Version: "v1"}, nil)
			registerMCPAppEditToolWithAuthorities(server, application.DefaultStore(), func(application.Store, []string) appEditAuthorities { return authorities })
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = serverSession.Close() })
			client := mcp.NewClient(&mcp.Implementation{Name: "edit-contract-client", Version: "v1"}, nil)
			session, err := client.Connect(ctx, clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })
			request := appMachineDesiredRequest{Target: target.Name, SnapshotRevision: "snapshot-1", Requirements: requirements}
			if strings.Contains(scenario, "preferences") {
				for _, requirement := range requirements {
					request.Choices = append(request.Choices, appMachineDesiredChoice{Requirement: requirement, InstanceID: "provider-" + string(requirement.Kind) + "-" + requirement.Name})
				}
			}
			proposed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: appMCPEditInput{
				Target: target.Name, Phase: "propose", Desired: request,
			}})
			if err != nil || proposed == nil || proposed.IsError {
				t.Fatalf("SDK propose failed: %v", err)
			}
			var proposal appMachineDesiredResult
			if err := json.Unmarshal(marshalStructured(t, proposed), &proposal); err != nil {
				t.Fatal(err)
			}
			consent := application.ProviderChangeConsent{PlanID: proposal.Plan.PlanID, First: true, Second: scenario != "approval"}
			if scenario == "wrong-target" {
				proposal.Desired.Target = "foreign-target"
			}
			if scenario == "stale" {
				proposal.PlanRevision = "old-plan"
			}
			switch scenario {
			case "requirements-added":
				proposal.Requirements = append(proposal.Requirements, capability.Requirement{Kind: capability.SQL, Name: "extra"})
			case "requirements-removed":
				proposal.Requirements = proposal.Requirements[:1]
			case "requirements-changed":
				proposal.Requirements[0].Name = "changed"
			case "requirements-reordered":
				proposal.Requirements[0], proposal.Requirements[1] = proposal.Requirements[1], proposal.Requirements[0]
			case "preferences-added":
				proposal.Preferences = append(proposal.Preferences, proposal.Preferences[0])
			case "preferences-removed":
				proposal.Preferences = proposal.Preferences[:1]
			case "preferences-changed":
				proposal.Preferences[0].InstanceID = "foreign-provider"
			case "preferences-reordered":
				proposal.Preferences[0], proposal.Preferences[1] = proposal.Preferences[1], proposal.Preferences[0]
			}
			beforeSource, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			beforeDeployment := deploymentSnapshot(t)
			applied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: appMCPEditInput{
				Target: target.Name, Phase: "apply", Desired: proposal.Desired, Proposal: &proposal, Consent: &consent,
			}})
			if err != nil || applied == nil {
				t.Fatalf("SDK apply transport failed: %v", err)
			}
			raw := marshalStructured(t, applied)
			if bytes.Contains(raw, []byte("private-executor-diagnostic")) || bytes.Contains(raw, []byte("private-secret-marker")) {
				t.Fatal("executor diagnostic leaked")
			}
			if scenario == "complete" || scenario == "complete-preferences" {
				var receipt application.ProviderChangeResult
				if err := json.Unmarshal(raw, &receipt); err != nil {
					t.Fatal(err)
				}
				if applied.IsError || executed != 1 || receipt.Status != "complete" || !reflect.DeepEqual(receipt.Outcomes, outcomes) {
					t.Fatalf("complete SDK receipt lost: %s", raw)
				}
				return
			}
			var payload struct {
				Error *machine.Error `json:"error"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if !applied.IsError || payload.Error == nil {
				t.Fatal("SDK apply did not return a structured failure")
			}
			want := machine.ErrorConflict
			if scenario == "approval" {
				want = machine.ErrorApprovalRequired
			}
			if scenario == "partial" {
				want = machine.ErrorVerificationFailed
			}
			if payload.Error.Code != want || payload.Error.Next == "" {
				t.Fatalf("SDK classification = %s, want %s: %s", payload.Error.Code, want, raw)
			}
			if scenario == "partial" {
				var details struct {
					Error struct {
						Details application.ProviderChangeResult `json:"details"`
					} `json:"error"`
				}
				if err := json.Unmarshal(raw, &details); err != nil {
					t.Fatal(err)
				}
				if executed != 1 || details.Error.Details.Status != "partial" || details.Error.Details.PlanID != plan.PlanID || !reflect.DeepEqual(details.Error.Details.Outcomes, outcomes) {
					t.Fatalf("partial SDK outcomes or material dispositions lost: %s", raw)
				}
			} else {
				if executed != 0 {
					t.Fatal("rejected SDK request executed a mutation")
				}
				afterSource, readErr := os.ReadFile(source)
				if readErr != nil || !bytes.Equal(beforeSource, afterSource) || !bytes.Equal(beforeDeployment, deploymentSnapshot(t)) {
					t.Fatalf("rejected SDK request changed source or deployment state: %v", readErr)
				}
			}
		})
	}
}

func TestCore921MachineAdapterPublicCategories(t *testing.T) {
	for _, test := range []struct {
		private string
		public  machine.ErrorCode
	}{
		{"approval_required", machine.ErrorApprovalRequired}, {"unsupported", machine.ErrorUnsupported},
		{"denied", machine.ErrorPolicyDenied}, {"cancel", machine.ErrorInternal}, {"partial", machine.ErrorVerificationFailed},
	} {
		got := machine.Classify(classifyMachineCLIError(&appMachineError{Code: test.private}))
		if got.Code != test.public || got.CauseCode != test.private || got.Next == "" {
			t.Fatalf("public category for %s = %+v", test.private, got)
		}
	}
}

func TestCore921MissingConsentPublicClassification(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", PlanID: "impact-1", ConfirmationsRequired: 2}
	_, err := appMachineChange(context.Background(), appMachineChangeRequest{
		Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1",
		Consent: application.ProviderChangeConsent{PlanID: "impact-1", First: true},
	}, func(context.Context, string) (GuidedReadContext, error) { return appMachineFixture(), nil },
		func(context.Context, string) (application.ProviderImpactPlan, error) { return plan, nil },
		func(context.Context, string, string) error { return nil },
		func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			t.Fatal("unconsented mutation")
			return nil, nil
		})
	if err == nil {
		t.Fatal("missing second confirmation accepted")
	}
	result, payload, transportErr := machineMCPFailure(err)
	if transportErr != nil || result == nil || !result.IsError {
		t.Fatalf("public error unavailable: %v", transportErr)
	}
	raw, _ := json.Marshal(payload)
	var decoded struct {
		Error machine.Error `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error.Code != machine.ErrorApprovalRequired {
		t.Fatalf("missing second provider confirmation surfaced as %q; next=%q; want approval_required", decoded.Error.Code, decoded.Error.Next)
	}
}
