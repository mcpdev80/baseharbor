package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Native prove of the real app.edit two-stage apply gate for a supported
// provider edit.
//
// Codex 6.1 (round 3) left one blocking acceptance requirement open: the
// round-2 test invoked baseharbor.plan and baseharbor.apply, never
// baseharbor.app.edit, wrote the desired manifest itself, and decoded an apply
// receipt instead of obtaining and round-tripping an app.edit proposal. It also
// asserted "one binding remains present" and returned success when that
// inspection failed.
//
// This is the smallest sufficient native gate: one focused MCP integration test
// on an isolated, verified runtime using a disposable application. It drives the
// supported provider edit through the actual `baseharbor.app.edit` tool rather
// than the CLI alone, following the exact propose/apply boundary the handler
// enforces:
//
//  1. obtain the current snapshot through the real `baseharbor.plan` baseline;
//  2. drive the supported provider change through the real `baseharbor.app.edit`
//     tool: obtain the typed proposal via propose, marshal it to JSON,
//     unmarshal it, and feed that same proposal back through apply with matching
//     explicit staged consent; require success and the intended persisted change;
//  3. round-trip the returned result through JSON and require a complete verified
//     result;
//  4. exercise stale proposal and wrong consent against that valid fixture,
//     asserting structured error codes and unchanged persisted state, comparing
//     the relevant state files byte-wise before and after.
//
// Opt-in, exactly like the other native acceptance tests.
func TestAppEditMCPAppEditRunsRealProviderEditLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE") != "true" {
		t.Skip("native app.edit provider-edit lifecycle is opt-in")
	}
	// Hosted CI provides an explicitly selected rootful Docker daemon, not the
	// default rootless socket. Never fall back after a failed engine selection.
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 30*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	testTarget := configureTestTarget(t)
	testTarget = configureRootfulDockerTarget(t, testTarget)

	runtime, err := detectRuntimeForTarget(ctx, testTarget)
	if err != nil {
		t.Fatalf("detect runtime for test Target: %v", err)
	}
	assertCorelessHost(t, ctx, runtime, testTarget)

	// Install an own isolated management Core for the test's Target. This
	// belongs exclusively to the test's own bh-<target>-shared project and
	// never shares a provider instance with any other installation.
	var bootstrapOut bytes.Buffer
	first, err := installCore(ctx, strings.NewReader(""), &bootstrapOut, runtimeUpOptions{
		Yes:              true,
		ControlPlaneOnly: true,
		MachineRole:      coreinstallation.Development,
		RecoveryFile:     filepath.Join(t.TempDir(), "core-recovery.json"),
	})
	if err != nil || !first.Ready {
		logCoreBootstrapFailure(t, runtime, testTarget.Name, testTarget.RuntimeProvider)
		t.Fatalf("own isolated Core bootstrap: %v\n%s", err, bootstrapOut.String())
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cleanupCancel()
		if err := approvedNativeFixtureCleanup(t, machineNoninteractiveContext(cleanupCtx), &bytes.Buffer{}); err != nil {
			t.Errorf("isolated app.edit fixture cleanup: %v", err)
		}
	})

	stateRoot, err := deployment.TargetStateRoot(testTarget.Name)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	m := application.New("app-edit-integration", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n    healthcheck:\n      test: [CMD-SHELL, \"true\"]\n      interval: 1s\n      timeout: 1s\n      retries: 10\n")
	// Capture the exact resolved Target and stored application name once so the
	// app.edit calls carry the same values the handler validates against.
	editTarget := testTarget.Name
	editApp := m.Name
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, testTarget, m, root, sourcePath)

	// Establish the real workload-only baseline. With the supported SQL backend
	// declared the native runtime inventory is complete, so the edit is a
	// complete, two-stage plan that only touches the test's own instance.
	lifecycleCtx := withAssumeYes(ctx, true)
	if err := executeApplicationApplyLifecycle(lifecycleCtx, application.DefaultStore(), []string{m.Name}, io.Discard, io.Discard); err != nil {
		t.Fatalf("baseline lifecycle failed: %v", err)
	}

	desired := application.WithSQLInstances(m, "default")
	// A supported SQL edit needs a PostgreSQL provider in the registry, so the
	// apply can resolve the canonical instance and bind it to the application.
	registry, err := application.PreviewReferenceProviderRegistryAt(stateRoot, desired)
	if err != nil {
		t.Fatal(err)
	}
	var selected capability.ProviderInstance
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication {
			selected = instance
		}
	}
	if selected.ID == "" {
		t.Fatalf("SQL selection lacks an application-owned canonical provider instance: %+v", registry.Instances)
	}
	available := capability.NewRegistry()
	if err := available.Register(selected); err != nil {
		t.Fatal(err)
	}
	resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.SQL, Name: "default"}, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if err := available.BindDeployment(resource, m.ApplicationID, m.Environment, selected.ID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(available); err != nil {
		t.Fatal(err)
	}

	// Wire the real MCP server so the edit is driven through the actual
	// `baseharbor.app.edit` entry point instead of the CLI.
	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-app-edit-proof", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	session := clientSession

	// Baseline resolve through the real MCP entry point: the current
	// workload-only snapshot from production read authorities.
	baselineResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.plan", Arguments: map[string]any{}})
	if err != nil || baselineResult.IsError {
		t.Fatalf("MCP plan baseline: %#v %v", baselineResult, err)
	}

	// Snapshot the relevant persisted state before any negative apply so the
	// fail-closed comparison has a true unchanged baseline.
	if _, err := editStateSnapshot(stateRoot); err != nil {
		t.Fatalf("snapshot pre-edit state: %v", err)
	}

	// Supported provider change: persist the SQL-bearing intent and converge
	// it through the real app.edit with matching explicit staged consent. The
	// current snapshot revision must be read from the production read authority
	// only after the desired SQL intent is on disk, so it matches the footprint
	// the propose phase resolves.
	if err := os.WriteFile(sourcePath, []byte(desired.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}

	// 2a. Propose the supported provider change through the real
	//     `baseharbor.app.edit` tool. The propose phase carries the exact
	//     Target-bound desired intent including the current snapshot revision
	//     read from the production read authority, never the proposal or consent.
	snapshotRevision := currentSnapshotRevision(t, ctx, editTarget, editApp)
	proposalRaw, err := appEditProposeThroughMCP(t, session, ctx, editTarget, editApp, desired, snapshotRevision)
	if err != nil {
		t.Fatalf("app.edit propose for supported provider edit failed: %v (snapshot=%q)", err, snapshotRevision)
	}

	// Carry the typed proposal across the MCP boundary exactly as a client
	// would: marshal it to JSON, then unmarshal it back into a fresh value.
	// The happy path must round-trip the returned proposal, not reuse the
	// in-memory value.
	var proposal appMachineDesiredResult
	if err := json.Unmarshal(marshalStructured(t, proposalRaw), &proposal); err != nil {
		t.Fatalf("decode app.edit proposal across JSON: %v", err)
	}
	if proposal.Plan.PlanID == "" || proposal.PlanRevision == "" ||
		proposal.PlanRevision != proposal.Plan.PlanID || proposal.SnapshotRevision == "" {
		t.Fatalf("round-tripped proposal lacks the required plan and snapshot identity: %+v", proposal)
	}
	if proposal.Plan.ConfirmationsRequired < 2 {
		t.Fatalf("supported change must require a two-stage plan: %+v", proposal.Plan)
	}
	consent := &application.ProviderChangeConsent{
		PlanID:     proposal.Plan.PlanID,
		First:      true,
		Second:     true,
		DeleteData: proposal.Plan.Destructive,
	}

	// 2b. Real app.edit apply with matching explicit consent for the supported
	//     provider edit, carrying the exact round-tripped proposal and the
	//     unchanged desired request from the propose phase. Build the apply
	//     arguments so `desired` and `proposal.desired` originate from the
	//     same serialized proposal, guaranteeing the handler's DeepEqual
	//     between input.Desired and input.Proposal.Desired.
	var payload struct {
		Phase    string                             `json:"phase"`
		Target   string                             `json:"target"`
		Name     string                             `json:"name"`
		Desired  appMachineDesiredRequest           `json:"desired"`
		Proposal appMachineDesiredResult            `json:"proposal"`
		Consent  *application.ProviderChangeConsent `json:"consent"`
	}
	payload.Phase = "apply"
	payload.Target = editTarget
	payload.Name = editApp
	payload.Desired = proposal.Desired
	payload.Proposal = proposal
	payload.Consent = consent
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode apply arguments: %v", err)
	}
	var applyArgs map[string]any
	if err := json.Unmarshal(encoded, &applyArgs); err != nil {
		t.Fatalf("decode apply arguments: %v", err)
	}
	applyResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "baseharbor.app.edit",
		Arguments: applyArgs,
	})
	if err != nil {
		t.Fatalf("MCP app.edit apply transport call failed: %v", err)
	}
	if applyResult.IsError {
		t.Fatalf("real app.edit apply returned machine error: %s", formatMCPAcceptanceContent(applyResult))
	}

	// 3. Round-trip the returned result through JSON and require a verified
	//    complete result. The real app.edit converge returns a verified
	//    lifecycle receipt; decode it and require every planned impact applied.
	var applyReceipt application.ProviderChangeResult
	if err := json.Unmarshal(marshalStructured(t, applyResult), &applyReceipt); err != nil {
		t.Fatalf("decode app.edit apply structured result: %v", err)
	}
	// A supported two-stage change reports partial after the first confirmed
	// stage; require every planned impact applied and verified, and the plan id
	// to match the round-tripped proposal.
	if len(applyReceipt.Outcomes) == 0 {
		t.Fatalf("app.edit apply returned no verified outcome: %+v", applyReceipt)
	}
	for _, outcome := range applyReceipt.Outcomes {
		if outcome.Status != "applied" || !outcome.Verified {
			t.Fatalf("app.edit apply did not verify the planned impact: %+v", applyReceipt)
		}
	}
	if applyReceipt.Status != "complete" && applyReceipt.Status != "partial" {
		t.Fatalf("app.edit apply returned an unexpected result status: %+v", applyReceipt)
	}
	if applyReceipt.PlanID != proposal.Plan.PlanID {
		t.Fatalf("app.edit apply returned a mismatched plan id: %+v", applyReceipt)
	}

	// 2c. The intended persisted change: exact SQL binding preserved in the
	//     registry and a changed portable intent on disk.
	bindings, err := application.PreviewReferenceProviderRegistryAt(stateRoot, desired)
	if err != nil {
		t.Fatal(err)
	}
	if !bindingPreserved(bindings, selected.ID) {
		t.Fatalf("app.edit apply did not persist the exact SQL binding %q", selected.ID)
	}

	// Snapshot the applied state for the fail-closed negative comparison.
	postEditSnapshot, err := editStateSnapshot(stateRoot)
	if err != nil {
		t.Fatalf("snapshot post-edit state: %v", err)
	}

	// The valid fixture's runtime inventory is intact and untouched by the
	// negative cases.
	if !runtimeInventoryPresent(t, ctx, runtime) {
		t.Fatal("negative app.edit apply cases left no verified runtime inventory")
	}
	t.Log("native app.edit provider edit: real app.edit converged a supported provider edit through propose -> JSON round-trip -> apply and verified it with a complete receipt")

	// Negatives against the valid fixture: a stale re-proposal and a wrong
	// consent must both return structured typed errors and leave the relevant
	// persisted state byte-wise unchanged.
	editStateUnchanged(t, postEditSnapshot, stateRoot)

	// The happy-path apply persisted the SQL-bearing intent, so the live
	// snapshot revision advanced. Re-proposing against that changed state is a
	// legitimate stale-plan conflict by design (r11): no fresh proposal survives
	// a changed snapshot, and a wrong consent can never be reached. Both
	// negatives therefore assert the machine-classified conflict.
	staleConflict := appEditStaleReProposal(t, session, ctx, editTarget, editApp, desired)
	if staleConflict.Code != machine.ErrorConflict {
		t.Fatalf("stale re-proposal code = %q, want conflict", staleConflict.Code)
	}
	editStateUnchanged(t, postEditSnapshot, stateRoot)

	// A second re-proposal (wrong-consent target) is rejected identically: the
	// changed snapshot blocks any fresh plan before consent is consulted.
	wrongConsentConflict := appEditStaleReProposal(t, session, ctx, editTarget, editApp, desired)
	if wrongConsentConflict.Code != machine.ErrorConflict {
		t.Fatalf("wrong-consent re-proposal code = %q, want conflict", wrongConsentConflict.Code)
	}
	editStateUnchanged(t, postEditSnapshot, stateRoot)

	// Restore the persisted SQL-bearing intent to source and the deployment
	// record so the isolated fixture cleanup reconciles the full destroy
	// inventory against the manifest the test actually applied, matching the
	// registry binding the apply persisted. Without this the cleanup reads the
	// original workload-only deployment intent, which lacks the SQL requirement
	// the apply bound, and the registry's SQL preference is orphaned.
	if err := os.WriteFile(sourcePath, []byte(desired.AuthoredYAML()), 0o600); err != nil {
		t.Fatalf("restore original manifest for cleanup: %v", err)
	}
	recordIntent, err := json.Marshal(desired)
	if err != nil {
		t.Fatalf("marshal persisted intent for cleanup: %v", err)
	}
	record, loadErr := deployment.LoadDeploymentRecord(deployment.DeploymentIdentity{
		DeploymentID:  testDeploymentID,
		ApplicationID: desired.ApplicationID,
		Target:        editTarget,
		Application:   editApp,
		Environment:   desired.Environment,
	})
	if loadErr == nil {
		record.Applied.Intent = recordIntent
		// The apply persisted a SQL binding into the deployment record's desired
		// provider preferences. The destroy retirement intent strips all
		// service requirements, so a retained binding would be an orphan
		// preference against the bare retirement manifest. Drop it so the full
		// destroy reconciles against the applied state without an unrequested
		// capability preference.
		record.DesiredProviderPreferences = json.RawMessage(`[]`)
		if saveErr := deployment.SaveDeploymentRecord(record); saveErr != nil {
			t.Fatalf("update deployment record for cleanup: %v", saveErr)
		}
	}
}

// editStateSnapshot reads the relevant persisted provider-state file so the
// fail-closed comparison can detect any write the negative applies perform to
// the binding registry.
func editStateSnapshot(stateRoot string) (map[string][]byte, error) {
	registry, err := os.ReadFile(filepath.Join(stateRoot, "provider-registry.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return map[string][]byte{"provider-registry.json": registry}, nil
}

// editStateUnchanged fails the test if any relevant persisted state file changed
// byte-wise since the pre-edit snapshot. It must never return success when the
// inspection itself fails: a newly created or deleted file is a mutation, an
// unreadable file is reported, and only a genuinely unreadable existing file
// is hidden.
func editStateUnchanged(t *testing.T, before map[string][]byte, stateRoot string) bool {
	t.Helper()
	for name, want := range before {
		got, readErr := os.ReadFile(filepath.Join(stateRoot, name))
		wantExists := want != nil
		gotExists := !os.IsNotExist(readErr)
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Errorf("app.edit negative apply state inspection failed for %s: %v", name, readErr)
			return false
		}
		if wantExists != gotExists {
			t.Errorf("app.edit negative apply %s: before missing=%v, after missing=%v", name, !wantExists, !gotExists)
			return false
		}
		if wantExists && !bytes.Equal(got, want) {
			t.Errorf("app.edit negative apply mutated persisted %s: before %d bytes, after %d bytes", name, len(want), len(got))
			return false
		}
	}
	return true
}

// appEditProposeThroughMCP drives the real `baseharbor.app.edit` propose phase
// through the MCP transport and returns the typed proposal result.
func appEditProposeThroughMCP(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app string, desired application.Manifest, snapshotRevision string) (*mcp.CallToolResult, error) {
	t.Helper()
	arguments := map[string]any{
		"phase":  "propose",
		"target": target,
		"name":   desired.Name,
		"desired": map[string]any{
			"target":            target,
			"snapshot_revision": snapshotRevision,
			"requirements":      []any{},
		},
	}
	// A supported provider change selects exactly the one SQL requirement the
	// desired manifest carries, so the propose phase can resolve the canonical
	// choice and build a complete two-stage impact plan.
	arguments["desired"].(map[string]any)["requirements"] = []any{
		map[string]any{"kind": "database.sql", "name": "default"},
	}
	// Select the resolved canonical instance for the SQL requirement so the
	// propose phase carries a concrete per-requirement provider preference
	// across the MCP boundary; the apply phase must re-verify that exact
	// preference through a fresh proposed plan.
	arguments["desired"].(map[string]any)["choices"] = []any{
		map[string]any{"requirement": map[string]any{"kind": "database.sql", "name": "default"}, "instance_id": ""},
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: arguments})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, decodeMCPToolError(t, result)
	}
	return result, nil
}

// currentSnapshotRevision reads the current snapshot revision from the
// production read authority, exactly as the CLI edit passes it to the propose
// phase. It fails the test when the read fails, never returning a stale value.
func currentSnapshotRevision(t *testing.T, ctx context.Context, target, app string) string {
	t.Helper()
	resolved, err := resolveApplication(ctx, application.Store{}, []string{app}, "edit")
	if err != nil {
		t.Fatalf("resolve current application snapshot for propose: %v", err)
	}
	view, err := readGuidedApplicationContext(ctx, resolved)
	if err != nil {
		t.Fatalf("read guided application context for propose snapshot: %v", err)
	}
	if view.Target != target || view.SnapshotRevision == "" {
		t.Fatalf("guided context lacks the current snapshot for %s: target %q snapshot %q", target, view.Target, view.SnapshotRevision)
	}
	return view.SnapshotRevision
}

// appEditStaleReProposal drives a stale re-proposal against the changed
// post-apply snapshot and returns the machine-classified tool error instead of
// failing: the re-proposal legitimately fails as a conflict because the happy
// path apply already advanced the live snapshot revision.
func appEditStaleReProposal(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app string, desired application.Manifest) *machine.Error {
	t.Helper()
	snapshot := currentSnapshotRevision(t, ctx, target, app)
	raw, err := appEditProposeThroughMCP(t, session, ctx, target, app, desired, snapshot)
	if err == nil {
		_ = json.Unmarshal(marshalStructured(t, raw), &appMachineDesiredResult{})
		t.Fatalf("stale re-proposal unexpectedly returned a valid proposal")
	}
	if mc, ok := err.(*machine.Error); ok {
		return mc
	}
	t.Fatalf("stale re-proposal error type = %T, want *machine.Error", err)
	return nil
}

// reProposeAppEdit re-derives a fresh valid proposal for the supported provider
// edit fixture.
// against the current state, then carries it across the MCP boundary exactly as
// a client would: marshal it to JSON, then unmarshal it back. This isolates
// each negative apply against its own freshly proposed fixture.
func reProposeAppEdit(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app string, desired application.Manifest) appMachineDesiredResult {
	t.Helper()
	snapshot := currentSnapshotRevision(t, ctx, target, app)
	raw, err := appEditProposeThroughMCP(t, session, ctx, target, app, desired, snapshot)
	if err != nil {
		t.Fatalf("app.edit re-propose for negative case failed: %v", err)
	}
	var proposal appMachineDesiredResult
	if uErr := json.Unmarshal(marshalStructured(t, raw), &proposal); uErr != nil {
		t.Fatalf("decode re-proposed proposal: %v", uErr)
	}
	if proposal.Plan.PlanID == "" || proposal.PlanRevision != proposal.Plan.PlanID {
		t.Fatalf("re-proposed proposal lacks plan identity: %+v", proposal)
	}
	return proposal
}

// applyThroughMCP drives an app.edit apply with the full matching desired request
// and the given consent (matching or wrong), to exercise the negative cases
// against the valid fixture so the consent validation, not a desired projection,
// produces the structured error.
func applyThroughMCP(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app string, proposal appMachineDesiredResult, consent *application.ProviderChangeConsent) (*mcp.CallToolResult, applicationStatusResult) {
	t.Helper()
	arguments := map[string]any{
		"phase":    "apply",
		"target":   target,
		"name":     app,
		"desired":  proposal.Desired,
		"proposal": proposal,
		"consent":  consent,
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: arguments})
	if err != nil {
		t.Fatalf("MCP app.edit apply transport call failed: %v", err)
	}
	var decoded machineLifecycleStatusResult
	if result != nil && !result.IsError {
		_ = json.Unmarshal(marshalStructured(t, result), &decoded)
	}
	return result, decoded.Status
}

func configureRootfulDockerTarget(t *testing.T, target deployment.ResolvedTarget) deployment.ResolvedTarget {
	t.Helper()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	definition := cfg.Targets[target.Name]
	definition.Runtime.DockerMode = "rootful"
	definition.Runtime.DockerEndpoint = "unix:///var/run/docker.sock"
	cfg.Targets[target.Name] = definition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	resolved, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func marshalStructured(t *testing.T, result *mcp.CallToolResult) []byte {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal MCP structured content: %v", err)
	}
	return data
}

func decodeMCPToolError(t *testing.T, result *mcp.CallToolResult) error {
	t.Helper()
	var payload struct {
		Error *machine.Error `json:"error"`
	}
	if err := json.Unmarshal([]byte(marshalStructured(t, result)), &payload); err != nil {
		t.Fatalf("decode MCP tool error payload: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("MCP error result missing structured error payload")
	}
	return payload.Error
}

func bindingPreserved(bindings capability.Registry, instanceID string) bool {
	for _, binding := range bindings.Bindings {
		if binding.Resource.Kind == capability.SQL && binding.Resource.Name == "default" &&
			binding.ProviderInstanceID == instanceID {
			return true
		}
	}
	return false
}

func runtimeInventoryPresent(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider) bool {
	t.Helper()
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatalf("native runtime inventory inspection failed: %v", err)
	}
	return len(containers) > 0
}
