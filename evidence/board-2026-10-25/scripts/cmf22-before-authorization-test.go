package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Blocking finding R1 regressions from the CMF20 independent review evidence
// (evidence/board-2026-10-25/codex-verdict-1077-25b46572-r2.md), ported from
// evidence/board-2026-10-25/scripts/cmf20-neutral-head-test.go and
// cmf20-overlay-test.go with the same assertions against the production tree.

// R1: an unmeasured registry-only preview projection can never authorize
// actual runtime-retirement verification (fail-closed like the 2f9afafd
// baseline), even when the convenience wrapper discards the diagnostics.
func TestCMF20RegistryProjectionCannotVerifyRuntimeRetirement(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.New("cmf20-neutral", "dev", true, false, false)
	manifest, err := application.WithBackendPlacementProjection(manifest, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	instanceID := "postgresql/application/" + manifest.ApplicationID + "/" + application.ProviderPlacementNameToken(manifest.Environment) + "/" + application.ProviderPlacementNameToken("default")
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{ID: instanceID, Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: manifest.ApplicationID, OwnerApplication: manifest.Name, OwnerEnvironment: manifest.Environment}); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, manifest.ApplicationID, manifest.Environment, instanceID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
	target.RuntimeProvider = "cmf20-unsupported-runtime"
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: manifest, FromRepository: true, Store: application.Store{Root: filepath.Join(t.TempDir(), "state"), Namespace: target.Name}}
	observed := observeApplicationProviderFacts(context.Background(), resolved)
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: target.Name, Complete: true, Changes: []application.ProviderChange{{InstanceID: instanceID, Ownership: capability.OwnershipBaseHarbor, Action: "stop"}}}
	if err := verifyApplicationRuntimeRetirement(resolved, plan, observed); err == nil {
		t.Fatalf("unmeasured registry-only projection authorized actual retirement verification: %+v", observed[instanceID])
	}
}

// R1: the wrapper/retirement regression on the wizard fixture — the projection
// remains present as preview evidence but never verifies retirement.
func TestCMF20PreviewProjectionMustNotVerifyRuntimeRetirement(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf20-unsupported-runtime"
	facts, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
	if !containsCauseCode(diagnostic, causeRuntimeInventoryNotYetMaterialized) {
		t.Fatalf("fixture must yield typed diagnostic: %v", diagnostic)
	}
	_, registry := cmf19Registry(t, resolved)
	id := registry.Instances[0].ID
	if len(facts) == 0 {
		t.Fatal("fixture must contain the projection")
	}
	consumed := observeApplicationProviderFacts(context.Background(), resolved)
	plan := application.ProviderPlan{Changes: []application.ProviderChange{{InstanceID: id, Action: "stop"}}}
	if err := verifyApplicationRuntimeRetirement(resolved, plan, consumed); err == nil {
		t.Fatalf("registry-only projection passed actual runtime-retirement verification after wrapper discarded diagnostic: fact=%+v diagnostic=%v", consumed[id], diagnostic)
	}
}

// R1: the real ownership caller must preserve the typed materialization
// diagnostic through machine.ResultError serialization, not only in the Go
// error chain (machine.Error.Cause is json:"-").
func TestCMF20RetainedOwnershipMachineResultMustPreserveDiagnostic(t *testing.T) {
	resolved := cmf19Fixture(t)
	resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
	store, registry := cmf19Registry(t, resolved)
	bound := registry.Instances[0]
	retained := capability.ProviderInstance{
		ID:                 application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, "dev", "retained"),
		Provider:           capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Scope:              capability.ScopeApplication,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: resolved.Manifest.ApplicationID,
		OwnerApplication:   resolved.Manifest.Name,
		OwnerEnvironment:   "dev",
	}
	if err := registry.Register(retained); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: resolved.Manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, resolved.Manifest.ApplicationID, "dev", bound.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	err := checkRetainedApplicationProviderOwnership(context.Background(), resolved)
	if err == nil {
		t.Fatal("retained unbound instance authorized although only a preview diagnostic exists")
	}
	if !containsCauseCode(err, "retained_provider_ownership_unverified") {
		t.Fatalf("ownership error lost its machine cause: %v", err)
	}
	if !containsCauseCode(err, causeRuntimeInventoryNotYetMaterialized) {
		t.Fatalf("caller lost the propagated materialization diagnostic: %v", err)
	}
	raw, marshalErr := json.Marshal(machine.ResultError(err))
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !bytes.Contains(raw, []byte(causeRuntimeInventoryNotYetMaterialized)) {
		t.Fatalf("machine result lost nested materialization diagnostic: %s", raw)
	}
}

// R1 test duty: the actual New/Edit wizard preview footprint output must show
// the projected instance as a preview (stopped-owned with unknown container
// evidence), never as a measured zero.
func TestCMF20WizardPreviewFootprintKeepsProjectionPreviewOnly(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf20-unsupported-runtime"
	footprint, err := resolveApplicationLifecycleFootprintWithFacts(resolved, observeApplicationProviderFacts(context.Background(), resolved))
	if err != nil {
		t.Fatalf("wizard preview footprint: %v", err)
	}
	_, registry := cmf19Registry(t, resolved)
	id := registry.Instances[0].ID
	found := false
	for _, entry := range append(append([]application.ProviderFootprint{}, footprint.Additional...), footprint.Existing...) {
		if entry.InstanceID != id {
			continue
		}
		found = true
		if entry.State != application.ProviderStoppedOwned {
			t.Fatalf("wizard preview lost its stopped-owned projection state: %+v", entry)
		}
		if entry.Containers.Classification == "measured" {
			t.Fatalf("wizard preview footprint claims a measured container count from a registry projection: %+v", entry.Containers)
		}
	}
	if !found {
		t.Fatalf("wizard preview footprint lost the projected instance %q: %+v", id, footprint)
	}
}

// R1 test duty: the lifecycle retirement output path preserves the typed
// materialization diagnostic in the real machine.JSON result alongside the
// fail-closed verification cause.
func TestCMF20RetirementMachineResultMustPreserveDiagnostic(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf20-unsupported-runtime"
	observed, diagnostics := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
	_, registry := cmf19Registry(t, resolved)
	id := registry.Instances[0].ID
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true, Changes: []application.ProviderChange{{InstanceID: id, Ownership: capability.OwnershipBaseHarbor, Action: "stop"}}}
	err := withMaterializationDiagnostics(verifyApplicationRuntimeRetirement(resolved, plan, observed), diagnostics)
	if err == nil {
		t.Fatal("projected facts verified runtime retirement")
	}
	if !containsCauseCode(err, "runtime_retirement_unverified") {
		t.Fatalf("retirement error lost its machine cause: %v", err)
	}
	raw, marshalErr := json.Marshal(machine.ResultError(err))
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !bytes.Contains(raw, []byte(causeRuntimeInventoryNotYetMaterialized)) {
		t.Fatalf("retirement machine result lost the materialization diagnostic: %s", raw)
	}
}


func TestCMF22NeutralProjectionCannotAuthorizeProviderRetirement(t *testing.T) {
 target:=configureTestTarget(t)
 manifest:=application.New("cmf20-neutral","dev",true,false,false)
 manifest,err:=application.WithBackendPlacementProjection(manifest,map[capability.ProviderKind]capability.ProviderPlacement{
  capability.ProviderPostgreSQL:{Scope:capability.ScopeApplication,Ownership:capability.OwnershipBaseHarbor},
 })
 if err!=nil {t.Fatal(err)}
 root,err:=deployment.TargetStateRoot(target.Name)
 if err!=nil {t.Fatal(err)}
 instanceID:="postgresql/application/"+manifest.ApplicationID+"/"+application.ProviderPlacementNameToken(manifest.Environment)+"/"+application.ProviderPlacementNameToken("default")
 registry:=capability.NewRegistry()
 if err:=registry.Register(capability.ProviderInstance{ID:instanceID,Provider:capability.Provider{Kind:capability.ProviderPostgreSQL,Capabilities:[]capability.Kind{capability.SQL}},Scope:capability.ScopeApplication,Ownership:capability.OwnershipBaseHarbor,OwnerApplicationID:manifest.ApplicationID,OwnerApplication:manifest.Name,OwnerEnvironment:manifest.Environment});err!=nil {t.Fatal(err)}
 if err:=registry.BindDeployment(capability.Resource{Application:manifest.Name,Kind:capability.SQL,Name:"default",Provider:capability.ProviderPostgreSQL},manifest.ApplicationID,manifest.Environment,instanceID);err!=nil {t.Fatal(err)}
 if err:=(capability.RegistryStore{Path:filepath.Join(root,"provider-registry.json")}).Save(registry);err!=nil {t.Fatal(err)}
 target.RuntimeProvider="cmf20-unsupported-runtime"
 resolved:=resolvedApplication{Target:target,TargetStateRoot:root,Manifest:manifest,FromRepository:true,Store:application.Store{Root:filepath.Join(t.TempDir(),"state"),Namespace:target.Name}}
 registerTestDeployment(t,target,manifest,t.TempDir(),t.TempDir())
 withdrawn:=resolved
 withdrawn.Manifest=applicationRuntimeRetirementIntent(resolved.Manifest)
 plan,err:=prepareApplicationProviderRetirement(context.Background(),withdrawn)
 if err==nil {t.Fatalf("registry-only projection passed retirement authorization with unavailable runtime: complete=%v changes=%+v",plan.Complete,plan.Changes)}
 t.Logf("fail-closed retirement authorization: %v",err)
}
