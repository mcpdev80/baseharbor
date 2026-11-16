package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func currentInstallationDestroyConsent(t *testing.T) context.Context {
	t.Helper()
	ctx := t.Context()
	targets, discovery := discoverFullDestroyTargets()
	if countFullDestroyBlockers(discovery) > 0 {
		t.Fatal(discovery)
	}
	plans, err := collectFullDestroyInventory(ctx, targets)
	if err != nil {
		t.Fatal(err)
	}
	preserved := []fullDestroyResult{}
	for _, target := range targets {
		item, err := preservedTargetRecovery(ctx, target)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status != "" {
			preserved = append(preserved, item)
		}
	}
	p, err := buildRuntimeDestroyImpact(ctx, plans, preserved, true)
	if err != nil {
		t.Fatal(err)
	}
	return withProviderChangeConsent(ctx, &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true})
}

func TestFullDestroyYesCannotReplacePlanOrDeletionConsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	root, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx := currentInstallationDestroyConsent(t)
	valid, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: valid.PlanID, First: true, DeleteData: true}, {PlanID: valid.PlanID, First: true, Second: true}, {PlanID: "stale", First: true, Second: true, DeleteData: true}} {
		var out bytes.Buffer
		err := runtimeDestroyAll(withProviderChangeConsent(t.Context(), ack), []string{"--all", "--yes"}, &out, &out)
		if err == nil {
			t.Fatalf("missing or stale consent deleted state: %s", out.String())
		}
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatalf("missing typed current destruction plan: %v", err)
		}
		if _, err := os.Stat(root); err != nil {
			t.Fatalf("consent denial changed ownership: %v", err)
		}
	}
}

func TestDestroyImpactChangesWithExactConfigBytes(t *testing.T) {
	coreStopFixture(t)
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	view := []targetDestroyInventory{{Target: target}}
	p, err := buildRuntimeDestroyImpact(t.Context(), view, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Scope.Destructive || p.Scope.Complete || p.Scope.ConfirmationsRequired != 2 {
		t.Fatal("incorrect destructive scope", p.Scope)
	}
	path, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := buildRuntimeDestroyImpact(t.Context(), view, nil, false)
	if err != nil || changed.Scope.PlanID == p.Scope.PlanID {
		t.Fatal("changed config accepted stale consent", err)
	}
}

func TestDestroyAuthorizesUnboundProviderWithoutMaterializedCore(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv(operatorauth.EnvIssuer, "")
	t.Setenv(operatorauth.EnvClientID, "")
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("retained-consumer", "prod", true, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	registry.Bindings = nil
	for i := range registry.Instances {
		registry.Instances[i].Scope = capability.ScopeApplication
		registry.Instances[i].SharingBoundary = ""
		registry.Instances[i].OwnerApplicationID = m.ApplicationID
		registry.Instances[i].OwnerApplication = m.Name
		registry.Instances[i].OwnerEnvironment = m.Environment
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		_, err := buildRuntimeDestroyImpact(t.Context(), []targetDestroyInventory{{Target: target}}, nil, all)
		if machine.Classify(err).Code != machine.ErrorAuthenticationFailed {
			t.Fatalf("unbound protected owner bypassed: %v", err)
		}
	}
	after, err := os.ReadFile(store.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed authorization changed ownership", err)
	}
}

func TestInstallationDestroyCLIJSONMCPShareScopeAndRejectAmbientConsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(providerChangeConsentEnv, `{"plan_id":"ambient","first":true,"second":true,"delete_data":true}`)
	var out bytes.Buffer
	err := runtimeDestroyCommand(withProviderChangeConsent(t.Context(), nil), []string{"--all", "--yes", "--json"}, &out, &out)
	var cliFailure *machine.Error
	if !errors.As(err, &cliFailure) || cliFailure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("CLI destruction bypass: %v", err)
	}
	var report destroyReport
	if json.Unmarshal(out.Bytes(), &report) != nil || report.Impact == nil || !report.Preview || report.Destroyed {
		t.Fatalf("missing JSON preview on consent denial: %s", out.String())
	}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "baseharbor.installation.destroy", Arguments: map[string]any{"approval": true}})
	if err != nil || !result.IsError {
		t.Fatalf("MCP bypassed plan consent: %+v %v", result, err)
	}
	raw, _ := json.Marshal(result.StructuredContent)
	var failure machineToolError
	if json.Unmarshal(raw, &failure) != nil || failure.Error == nil || failure.Error.Code != cliFailure.Code {
		t.Fatalf("MCP error drift: %s", raw)
	}
	planData, _ := json.Marshal(failure.Error.Details)
	var plan runtimeDestroyImpact
	if json.Unmarshal(planData, &plan) != nil || plan.Scope.PlanID != report.Impact.Scope.PlanID {
		t.Fatalf("CLI/JSON/MCP scope drift: %s", planData)
	}
}
