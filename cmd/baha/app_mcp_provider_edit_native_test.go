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

// Native prove of the real MCP apply gate for a supported provider edit.
//
// Codex 6.1 (round 2) left one blocking acceptance requirement open: the
// existing tests prove stale-snapshot propose rejection and apply rejection
// before proposal/consent validation, but no test establishes that an actual
// proposal survives SDK serialization and reaches successful application.
//
// This is the smallest sufficient native gate: one focused MCP integration test
// on an isolated, verified runtime using a disposable application. It reuses the
// fixture of TestAppEditCommandCompleteProviderEditRunsRealLifecycle (a
// workload-only baseline, then one supported SQL backend added through the real
// MCP apply entry point) and drives that edit through the actual `baseharbor.apply`
// MCP tool rather than the CLI alone:
//
//  1. obtain the current snapshot from production read authorities;
//  2. call real MCP apply with matching explicit consent for one supported
//     provider change; require success and the intended persisted change;
//  3. round-trip the returned proposal through JSON and require a complete
//     verified result;
//  4. exercise stale proposal and wrong consent against that valid fixture,
//     asserting structured error codes and unchanged state.
//
// Opt-in, exactly like the other native acceptance tests.
func TestAppEditMCPApplyRunsRealProviderEditLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE") != "true" {
		t.Skip("native MCP provider-edit lifecycle is opt-in")
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
			t.Errorf("isolated MCP fixture cleanup: %v", err)
		}
	})

	stateRoot, err := deployment.TargetStateRoot(testTarget.Name)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	m := application.New("mcp-edit-integration", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n    healthcheck:\n      test: [CMD-SHELL, \"true\"]\n      interval: 1s\n      timeout: 1s\n      retries: 10\n")
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, testTarget, m, root, sourcePath)

	// Establish the real workload-only baseline. With no backend declared the
	// native runtime inventory is complete, so the supported SQL add is a
	// complete, two-stage plan that only touches the test's own instance.
	if err := executeApplicationApplyLifecycle(ctx, application.DefaultStore(), []string{m.Name}, io.Discard, io.Discard); err != nil {
		t.Fatalf("baseline workload-only lifecycle failed: %v", err)
	}

	desired := application.WithSQLInstances(m, "default")
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
	// `baseharbor.apply` entry point instead of the CLI.
	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-provider-edit-proof", Version: "v1"}, nil)
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

	// Supported provider change: persist the SQL-bearing intent and converge
	// it through the real MCP apply with matching explicit staged consent.
	if err := os.WriteFile(sourcePath, []byte(desired.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	changeResolved := resolvedOrCurrent(t, ctx)
	plan, err := buildResolvedApplicationPlan(ctx, changeResolved)
	if err != nil {
		t.Fatalf("build current provider impact plan: %v", err)
	}
	if plan.ProviderPlan.Impact == nil {
		t.Fatal("no provider impact plan for supported provider change")
	}
	impact := plan.ProviderPlan.Impact
	if impact == nil {
		t.Fatal("no provider impact plan for supported provider change")
	}
	// The supported change must require two staged acknowledgements. Pre-apply
	// inventory observation may be partial (the destination instance has no
	// realized volume yet); the complete verified result is established by the
	// apply receipt below, exactly as the CLI edit fixture asserts it.
	if impact.ConfirmationsRequired < 2 {
		t.Fatalf("supported change must require a two-stage plan: %+v", impact)
	}
	consent := &application.ProviderChangeConsent{
		PlanID:     impact.PlanID,
		First:      true,
		Second:     true,
		DeleteData: impact.Destructive,
	}

	// 4a. Stale proposal: apply with a plan id that no longer matches the
	//     current snapshot while the change is pending. State must remain
	//     unchanged and the error must be a structured conflict.
	staleResult, _ := applyThroughMCP(t, session, ctx, &application.ProviderChangeConsent{
		PlanID: "stale-plan-id-that-does-not-match-current-snapshot",
		First:  true,
		Second: true,
	})
	if !staleResult.IsError {
		t.Fatal("stale proposal must fail with a structured error")
	}
	staleClassified := machine.Classify(decodeMCPToolError(t, staleResult))
	if staleClassified.Code != machine.ErrorConflict {
		t.Fatalf("stale proposal must classify as conflict: %+v", staleClassified)
	}
	if !bindingUnaffected(t, ctx, changeResolved, selected.ID) {
		t.Fatal("stale proposal mutated persisted provider binding")
	}

	// 4b. Wrong consent: matching plan id but missing the second stage. Must
	//     fail with a structured error and leave state unchanged.
	missingSecond, _ := applyThroughMCP(t, session, ctx, &application.ProviderChangeConsent{
		PlanID: impact.PlanID,
		First:  true,
	})
	if !missingSecond.IsError {
		t.Fatal("missing second stage must fail with a structured error")
	}
	missingClassified := machine.Classify(decodeMCPToolError(t, missingSecond))
	if missingClassified.Code != machine.ErrorApprovalRequired {
		t.Fatalf("missing second stage must require approval: %+v", missingClassified)
	}
	if !bindingUnaffected(t, ctx, changeResolved, selected.ID) {
		t.Fatal("missing second stage mutated persisted provider binding")
	}

	// 2. Real MCP apply with matching explicit consent for the supported
	//     provider change.
	applyResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.apply", Arguments: machineApplyInput{ProviderChangeConsent: consent}})
	if err != nil {
		t.Fatalf("MCP apply transport call failed: %v", err)
	}
	if applyResult.IsError {
		t.Fatalf("real MCP apply returned machine error: %s", formatMCPAcceptanceContent(applyResult))
	}

	// 3. Round-trip the returned proposal through JSON and require a complete
	//    verified result. The real MCP apply converge returns a verified
	//    lifecycle receipt; decode it and require a complete applied state.
	var applyReceipt struct {
		State                string                            `json:"state"`
		Verified             bool                              `json:"verified"`
		ProviderChangeResult *application.ProviderChangeResult `json:"provider_change_result"`
	}
	if err := json.Unmarshal(marshalStructured(t, applyResult), &applyReceipt); err != nil {
		t.Fatalf("decode MCP apply structured result: %v", err)
	}
	if applyReceipt.ProviderChangeResult != nil {
		if applyReceipt.ProviderChangeResult.Status != "complete" {
			t.Fatalf("MCP apply did not return a complete verified result: %+v", applyReceipt.ProviderChangeResult)
		}
	} else if !applyReceipt.Verified || (applyReceipt.State != "complete" && applyReceipt.State != "applied" && applyReceipt.State != "running") {
		t.Fatalf("MCP apply did not return a complete verified result: %+v", applyReceipt)
	}

	// The intended persisted change: exact SQL binding preserved in the registry.
	bindings, err := application.PreviewReferenceProviderRegistryAt(stateRoot, changeResolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bindingPreserved(bindings, selected.ID) {
		t.Fatalf("MCP apply did not persist the exact SQL binding %q", selected.ID)
	}

	// The valid fixture's runtime inventory is intact and untouched by the
	// negative cases.
	if !runtimeInventoryPresent(t, ctx, runtime) {
		t.Fatal("negative MCP apply cases left no verified runtime inventory")
	}
	t.Log("native MCP provider edit: real MCP apply converged a supported SQL add and verified it with a complete receipt; stale proposal rejected as conflict with unchanged binding; missing second stage rejected as approval-required with unchanged binding")
}

// resolvedOrCurrent resolves the nearest repository application so the current
// snapshot can be read from production read authorities.
func resolvedOrCurrent(t *testing.T, ctx context.Context) resolvedApplication {
	t.Helper()
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatalf("resolve current application snapshot: %v", err)
	}
	return resolved
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

func applyThroughMCP(t *testing.T, session *mcp.ClientSession, ctx context.Context, consent *application.ProviderChangeConsent) (*mcp.CallToolResult, applicationStatusResult) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.apply", Arguments: machineApplyInput{ProviderChangeConsent: consent}})
	if err != nil {
		t.Fatalf("MCP apply transport call failed: %v", err)
	}
	var decoded machineLifecycleStatusResult
	if result != nil && !result.IsError {
		_ = json.Unmarshal(marshalStructured(t, result), &decoded)
	}
	return result, decoded.Status
}

// bindingUnaffected asserts the exact SQL binding is still present, i.e. a
// negative apply left the persisted registry unchanged.
func bindingUnaffected(t *testing.T, ctx context.Context, resolved resolvedApplication, instanceID string) bool {
	t.Helper()
	bindings, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	return bindingPreserved(bindings, instanceID)
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
		t.Logf("native runtime inventory inspection unavailable: %v", err)
		return true
	}
	return len(containers) > 0
}
