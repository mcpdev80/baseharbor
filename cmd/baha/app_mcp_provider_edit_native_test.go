package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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

// This opt-in native gate drives a real provider edit through MCP, including
// JSON transport, explicit consent, verified convergence and write-free
// wrong-consent/stale-proposal rejection on the same isolated application.
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
	root := t.TempDir()
	recoveryPath := filepath.Join(t.TempDir(), "core-recovery.json")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cleanupCancel()
		if err := approvedNativeFixtureCleanup(t, machineNoninteractiveContext(cleanupCtx), &bytes.Buffer{}); err != nil {
			t.Errorf("isolated app.edit fixture cleanup: %v", err)
		}
	})

	// Install an own isolated management Core for the test's Target. This
	// belongs exclusively to the test's own bh-<target>-shared project and
	// never shares a provider instance with any other installation.
	var bootstrapOut bytes.Buffer
	first, err := installCore(ctx, strings.NewReader(""), &bootstrapOut, runtimeUpOptions{
		Yes:              true,
		ControlPlaneOnly: true,
		MachineRole:      coreinstallation.Development,
		RecoveryFile:     recoveryPath,
	})
	if err != nil || !first.Ready {
		logCoreBootstrapFailure(t, runtime, testTarget.Name, testTarget.RuntimeProvider)
		t.Fatalf("own isolated Core bootstrap: %v\n%s", err, bootstrapOut.String())
	}

	stateRoot, err := deployment.TargetStateRoot(testTarget.Name)
	if err != nil {
		t.Fatal(err)
	}
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

	// Establish the real workload-only baseline before proposing SQL intent.
	lifecycleCtx := withAssumeYes(ctx, true)
	if err := executeApplicationApplyLifecycle(lifecycleCtx, application.DefaultStore(), []string{m.Name}, io.Discard, io.Discard); err != nil {
		t.Fatalf("baseline lifecycle failed: %v", err)
	}

	desired := application.WithSQLInstances(m, "default")
	// Retain the canonical binding of a stopped, never-realized SQL provider.
	// The daemon still has no SQL volume; Edit must realize and verify it.
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

	baselineResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.plan", Arguments: map[string]any{}})
	if err != nil || baselineResult == nil || baselineResult.IsError {
		t.Fatalf("MCP plan baseline failed: %v", err)
	}
	oldSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := editStateSnapshot(stateRoot, root)
	if err != nil {
		t.Fatalf("snapshot pre-edit state: %v", err)
	}

	// Propose from the workload-only source. Only the real consented Apply
	// may persist SQL intent, preferences or deployment state.
	snapshotRevision := currentSnapshotRevision(t, ctx, editTarget, editApp)
	proposal := appEditProposeThroughMCP(t, session, ctx, editTarget, editApp, selected.ID, snapshotRevision)
	if proposal.ApplicationID != m.ApplicationID || proposal.Plan.Target != editTarget ||
		proposal.PlanRevision != proposal.Plan.PlanID || proposal.Plan.PlanID == "" ||
		proposal.SnapshotRevision != snapshotRevision || proposal.Desired.SnapshotRevision != snapshotRevision {
		t.Fatal("round-tripped proposal lacks the exact application, Target, plan or snapshot identity")
	}
	if !proposal.Plan.Complete || proposal.Plan.ConfirmationsRequired < 1 || proposal.Plan.ConfirmationsRequired > 2 {
		t.Fatalf("supported provider edit lacks a complete valid consent plan: complete=%t confirmations=%d impacts=%d",
			proposal.Plan.Complete, proposal.Plan.ConfirmationsRequired, len(proposal.Plan.Impacts))
	}
	editStateUnchanged(t, baseline, stateRoot, root)
	consent := application.ProviderChangeConsent{
		PlanID: proposal.Plan.PlanID, First: true, Second: true, DeleteData: proposal.Plan.Destructive,
	}

	// Carry the valid proposal to apply with a genuinely different consent.
	// A typed conflict must be returned before any persisted state changes.
	wrongConsent := consent
	wrongConsent.PlanID += "-wrong-consent"
	denied := applyThroughMCP(t, session, ctx, editTarget, editApp, proposal, wrongConsent)
	requireAppEditMCPConflict(t, denied)
	editStateUnchanged(t, baseline, stateRoot, root)

	applied := applyThroughMCP(t, session, ctx, editTarget, editApp, proposal, consent)
	if applied.IsError {
		t.Fatal("real app.edit apply returned a machine error")
	}
	var receipt application.ProviderChangeResult
	if err := json.Unmarshal(marshalStructured(t, applied), &receipt); err != nil {
		t.Fatalf("decode app.edit apply structured result: %v", err)
	}
	if receipt.Status != "complete" || receipt.PlanID != proposal.Plan.PlanID || len(receipt.Outcomes) == 0 {
		t.Fatalf("app.edit result is not a complete nonempty receipt: status=%q", receipt.Status)
	}
	verifiedSQL := false
	for _, outcome := range receipt.Outcomes {
		if outcome.Status != "applied" || !outcome.Verified || outcome.Evidence == "" {
			t.Fatal("app.edit did not verify every planned impact")
		}
		if outcome.InstanceID == selected.ID && (outcome.Operation == "add" || outcome.Operation == "resume") &&
			outcome.After != nil && outcome.After.Verified && outcome.After.State == "running" &&
			outcome.After.Target == editTarget && outcome.After.Owner == m.ApplicationID {
			verifiedSQL = true
		}
	}
	if !verifiedSQL {
		t.Fatal("app.edit lacks an authoritative running SQL add/resume receipt for this application and Target")
	}
	newSource, err := os.ReadFile(sourcePath)
	if err != nil || bytes.Equal(newSource, oldSource) {
		t.Fatalf("app.edit did not persist the changed portable intent: %v", err)
	}
	persisted, err := application.ParseYAML(string(newSource))
	if err != nil || persisted.AuthoredYAML() != desired.AuthoredYAML() {
		t.Fatalf("app.edit persisted different portable SQL intent: %v", err)
	}
	resolved, err := resolveApplication(ctx, application.DefaultStore(), []string{editApp}, "plan")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Load()
	if err != nil || !bindingPreserved(bindings, m, selected.ID) {
		t.Fatalf("app.edit did not retain the actual exact SQL binding: %v", err)
	}
	preferences, err := application.DesiredProviderPreferencesAt(stateRoot, resolved.Manifest)
	if err != nil || len(preferences) != 1 || preferences[0].InstanceID != selected.ID ||
		preferences[0].Requirement != (capability.Requirement{Kind: capability.SQL, Name: "default"}) {
		t.Fatalf("app.edit did not persist the exact Target provider preference: %v", err)
	}
	runtimeFiles, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := application.ObserveSQLConsumerInventory(ctx,
		application.NewRuntimeBackendProbeExecutor(runtime, runtimeFiles), runtimeFiles, resolved.Manifest, "default")
	if err != nil || !catalog.Empty || catalog.Consumer != "application/"+m.ApplicationID+"/"+m.Environment {
		t.Fatalf("actual application SQL access and empty catalog are unverified: %v", err)
	}
	if !runtimeInventoryPresent(t, ctx, runtime) {
		t.Fatal("native app.edit left no runtime inventory")
	}

	// Replay the original proposal against a genuinely changed live snapshot.
	// This must reach app.edit apply and reject the stale proposal unchanged.
	if currentSnapshotRevision(t, ctx, editTarget, editApp) == proposal.SnapshotRevision {
		t.Fatal("successful app.edit did not advance the live snapshot")
	}
	postEdit, err := editStateSnapshot(stateRoot, root)
	if err != nil {
		t.Fatalf("snapshot applied state: %v", err)
	}
	stale := applyThroughMCP(t, session, ctx, editTarget, editApp, proposal, consent)
	requireAppEditMCPConflict(t, stale)
	editStateUnchanged(t, postEdit, stateRoot, root)
	t.Log("native app.edit: complete verified SQL edit; wrong consent and stale apply rejected without persisted changes")
}

type appEditPersistedFile struct {
	Mode os.FileMode
	Data []byte
}

// Capture all Target-owned and repository canonical files as well as the
// authored source. Inspection errors and symlinks cannot count as unchanged.
func editStateSnapshot(stateRoot, repositoryRoot string) (map[string]appEditPersistedFile, error) {
	files := map[string]appEditPersistedFile{}
	roots := []string{stateRoot, filepath.Join(repositoryRoot, ".baseharbor"), filepath.Join(repositoryRoot, application.RepositoryManifestName)}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			item := appEditPersistedFile{Mode: info.Mode()}
			switch {
			case info.IsDir():
			case info.Mode().IsRegular():
				item.Data, err = os.ReadFile(path)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("cannot inspect non-regular persisted state %s", path)
			}
			files[path] = item
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func editStateUnchanged(t *testing.T, before map[string]appEditPersistedFile, stateRoot, repositoryRoot string) {
	t.Helper()
	after, err := editStateSnapshot(stateRoot, repositoryRoot)
	if err != nil {
		t.Fatalf("inspect persisted state after rejected edit: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected app.edit changed persisted Target, deployment, provider, canonical or authored state")
	}
}

func TestAppEditMCPNativeStateSnapshotDetectsMutationAndInspectionFailure(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "target")
	repository := filepath.Join(root, "repository")
	paths := []string{
		filepath.Join(state, "provider-registry.json"),
		filepath.Join(state, "deployments", testDeploymentID, "deployment.json"),
		filepath.Join(repository, ".baseharbor", "apps", "example", application.RepositoryManifestName),
		filepath.Join(repository, application.RepositoryManifestName),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := editStateSnapshot(state, repository)
	if err != nil {
		t.Fatal(err)
	}
	editStateUnchanged(t, baseline, state, repository)
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
			t.Fatal(err)
		}
		after, err := editStateSnapshot(state, repository)
		if err != nil || reflect.DeepEqual(baseline, after) {
			t.Fatalf("persisted mutation was not captured for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	created := filepath.Join(state, "new-state.json")
	if err := os.WriteFile(created, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := editStateSnapshot(state, repository)
	if err != nil || reflect.DeepEqual(baseline, after) {
		t.Fatalf("new persisted file was not captured: %v", err)
	}
	if err := os.Remove(created); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	after, err = editStateSnapshot(state, repository)
	if err != nil || reflect.DeepEqual(baseline, after) {
		t.Fatalf("deleted persisted file was not captured: %v", err)
	}
	if err := os.Symlink(paths[1], paths[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := editStateSnapshot(state, repository); err == nil {
		t.Fatal("uninspectable symlink must fail closed")
	}
}

// The returned proposal crosses a real JSON boundary before it is ever reused.
func appEditProposeThroughMCP(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app, instanceID, snapshotRevision string) appMachineDesiredResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: map[string]any{
		"phase": "propose", "target": target, "name": app,
		"desired": map[string]any{
			"target": target, "snapshot_revision": snapshotRevision,
			"requirements": []any{map[string]any{"kind": "database.sql", "name": "default"}},
			"choices": []any{map[string]any{
				"requirement": map[string]any{"kind": "database.sql", "name": "default"}, "instance_id": instanceID,
			}},
		},
	}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("real MCP app.edit propose failed: %v", err)
	}
	var proposal appMachineDesiredResult
	if err := json.Unmarshal(marshalStructured(t, result), &proposal); err != nil {
		t.Fatalf("decode app.edit proposal across JSON: %v", err)
	}
	return proposal
}

func currentSnapshotRevision(t *testing.T, ctx context.Context, target, app string) string {
	t.Helper()
	resolved, err := resolveApplication(ctx, application.DefaultStore(), []string{app}, "edit")
	if err != nil {
		t.Fatal(err)
	}
	view, err := readGuidedApplicationContext(ctx, resolved)
	if err != nil {
		t.Fatalf("read production snapshot for app.edit: %v", err)
	}
	if view.Target != target || view.SnapshotRevision == "" {
		t.Fatal("production snapshot lacks the exact Target and revision")
	}
	return view.SnapshotRevision
}

func applyThroughMCP(t *testing.T, session *mcp.ClientSession, ctx context.Context, target, app string, proposal appMachineDesiredResult, consent application.ProviderChangeConsent) *mcp.CallToolResult {
	t.Helper()
	encoded, err := json.Marshal(appMCPEditInput{
		Phase: "apply", Target: target, Name: app, Desired: proposal.Desired, Proposal: &proposal, Consent: &consent,
	})
	if err != nil {
		t.Fatal(err)
	}
	var arguments map[string]any
	if err := json.Unmarshal(encoded, &arguments); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.app.edit", Arguments: arguments})
	if err != nil || result == nil {
		t.Fatalf("real MCP app.edit apply transport failed: %v", err)
	}
	return result
}

func requireAppEditMCPConflict(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if !result.IsError {
		t.Fatal("invalid app.edit apply unexpectedly succeeded")
	}
	var payload struct {
		Error *machine.Error `json:"error"`
	}
	if err := json.Unmarshal(marshalStructured(t, result), &payload); err != nil {
		t.Fatalf("decode structured app.edit error: %v", err)
	}
	if payload.Error == nil || payload.Error.Code != machine.ErrorConflict || payload.Error.Next == "" || payload.Error.Message != "application machine: conflict" {
		t.Fatal("invalid app.edit apply did not return a typed conflict with remediation")
	}
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

func bindingPreserved(bindings capability.Registry, manifest application.Manifest, instanceID string) bool {
	for _, binding := range bindings.Bindings {
		if binding.Resource.Kind == capability.SQL && binding.Resource.Name == "default" &&
			binding.ProviderInstanceID == instanceID && binding.ApplicationID == manifest.ApplicationID &&
			binding.Environment == manifest.Environment && binding.Resource.Application == manifest.Name {
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
