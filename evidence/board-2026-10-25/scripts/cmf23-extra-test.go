package main

import (
"os"
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

func TestCMF20OwnerFileFailuresStayFailClosed(t *testing.T) {
 for _, mode := range []string{"missing-compose", "missing-env", "symlink-dir", "symlink-compose", "unprotected-dir", "unprotected-compose", "noncanonical-compose", "unreadable-dir"} {
  t.Run(mode,func(t *testing.T) {
   resolved := cmf19Fixture(t)
   cmf19BindAll(t,resolved)
   files := cmf19MaterializeCanonicalFiles(t,resolved)
   switch mode {
   case "missing-compose": if err:=os.Remove(files.Compose);err!=nil{t.Fatal(err)}
   case "missing-env": if err:=os.Remove(files.Env);err!=nil{t.Fatal(err)}
   case "symlink-dir":
    target:=files.Dir+"-real"
    if err:=os.Rename(files.Dir,target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,files.Dir);err!=nil{t.Fatal(err)}
   case "symlink-compose":
    target:=files.Compose+"-real"
    if err:=os.Rename(files.Compose,target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,files.Compose);err!=nil{t.Fatal(err)}
   case "unprotected-dir": if err:=os.Chmod(files.Dir,0777);err!=nil{t.Fatal(err)}
   case "unprotected-compose": if err:=os.Chmod(files.Compose,0666);err!=nil{t.Fatal(err)}
   case "noncanonical-compose": if err:=os.WriteFile(files.Compose,[]byte("services: {}\n"),0600);err!=nil{t.Fatal(err)}
   case "unreadable-dir":
    if err:=os.Chmod(files.Dir,0000);err!=nil{t.Fatal(err)}
    t.Cleanup(func(){os.Chmod(files.Dir,0700)})
   }
   resolved.Target.RuntimeProvider="cmf20-unsupported-runtime"
   facts:=map[string]application.ProviderFact{}
   diagnostic:=observeNativeBackendProviderFacts(context.Background(),resolved,facts)
   if len(facts)!=0 || diagnostic!=nil {t.Fatalf("owner-file failure manufactured projection: facts=%+v diagnostic=%v",facts,diagnostic)}
  })
 }
}

// ROLLE: REVIEW. Append via overlay to cmf20_r1_test.go; product sources stay unchanged.
func TestCMF21StructuredCLIJSONRetainsMaterializationContext(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf21-unsupported-runtime"
	observed, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
	_, registry := cmf19Registry(t, resolved)
	id := registry.Instances[0].ID
	fact, found := observed[id]
	if !found || fact.Containers.Value != nil || fact.Containers.Classification != "unknown" {
		t.Fatalf("projection must have unknown container quantity: %+v", fact)
	}
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true,
		Changes: []application.ProviderChange{{InstanceID: id, Ownership: capability.OwnershipBaseHarbor, Action: "stop"}}}
	retirement := withMaterializationDiagnostics(verifyApplicationRuntimeRetirement(resolved, plan, observed), diagnostic)
	if retirement == nil { t.Fatal("preview authorized retirement") }
	// Force the ordinary retained-owner rejection while retaining the bound
	// instance that produces the materialization diagnostic.
	store, registry := cmf19Registry(t, resolved)
	retained := registry.Instances[0]
	retained.ID = application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, resolved.Manifest.Environment, "retained")
	if err := registry.Register(retained); err != nil { t.Fatal(err) }
	if err := store.Save(registry); err != nil { t.Fatal(err) }
	ownership := checkRetainedApplicationProviderOwnership(context.Background(), resolved)
	if ownership == nil { t.Fatal("unverified retained owner authorized") }
	for name, resultErr := range map[string]error{"retirement": retirement, "ownership": ownership} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			// Match main.go's real CLI classification and JSON encoder.
			if err := writeJSON(&out, machine.ResultError(classifyMachineCLIError(resultErr))); err != nil { t.Fatal(err) }
			var decoded struct { Error struct {
				Cause string `json:"cause"`
				Details struct { Diagnostics []struct {
					Code string `json:"code"`
					Cause string `json:"cause"`
					Resource string `json:"resource"`
					Details struct { Target string `json:"target"`; Instance string `json:"instance"`; ApplicationID string `json:"application_id"` } `json:"details"`
				} `json:"materialization_diagnostics"` } `json:"details"`
			} `json:"error"` }
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil { t.Fatal(err) }
			want := "runtime_retirement_unverified"
			if name == "ownership" { want = "retained_provider_ownership_unverified" }
			if decoded.Error.Cause != want { t.Fatalf("outer cause: %s", out.String()) }
			if len(decoded.Error.Details.Diagnostics) == 0 { t.Fatalf("missing structured diagnostic: %s", out.String()) }
			for _, d := range decoded.Error.Details.Diagnostics {
			if d.Code != string(machine.ErrorRuntimeUnavailable) || d.Cause != causeRuntimeInventoryNotYetMaterialized || d.Resource != id || d.Details.Instance != id || d.Details.Target != resolved.Target.Name || d.Details.ApplicationID != resolved.Manifest.ApplicationID {
				t.Fatalf("lost typed context: %s", out.String())
			}
			}
			t.Logf("serialized CLI machine result: %s", out.String())
		})
	}
}
