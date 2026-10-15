package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func backendBindingFixture(t *testing.T, kind capability.ProviderKind) resolvedApplication {
	t.Helper()
	configureTestTarget(t)
	t.Setenv(application.ProviderScopeEnv(kind), "shared")
	t.Chdir(httpsAdoptionFixture(t, false))
	m := application.WithWorkloadComponents(application.New("demo", "dev", kind == capability.ProviderPostgreSQL, kind == capability.ProviderValkey, false), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	r, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), r, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBackendBindingReplacementPlanRetainsOriginalOwnerAndDescribesDataLoss(t *testing.T) {
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		t.Run(string(kind), func(t *testing.T) {
			r := backendBindingFixture(t, kind)
			path := filepath.Join(r.TargetStateRoot, "provider-registry.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := (capability.RegistryStore{Path: path}).Load()
			if err != nil || len(actual.Bindings) != 1 {
				t.Fatal("missing original binding", err)
			}
			old := actual.Bindings[0].ProviderInstanceID
			t.Setenv(application.ProviderScopeEnv(kind), "application")
			r = captureBackendSelectionForTest(t, r)
			module := application.SharedBackendRuntimeDependency(r.Target.Name, kind)
			facts := map[string]application.ProviderFact{
				old:       {Target: r.Target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{module.ID}},
				module.ID: {Target: r.Target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "runtime/shared"},
			}
			snapshot, preferences, err := applicationLifecycleSnapshot(r, facts)
			if err != nil {
				t.Fatal("supported placement replacement still refused", err)
			}
			fp, err := application.ResolveFootprint(r.Manifest, snapshot, preferences)
			if err != nil || len(fp.Requirements) != 1 || fp.Requirements[0].InstanceID == old || fp.Requirements[0].Binding != "requested/unbound" {
				t.Fatalf("proposed destination became an applied binding: %+v %v", fp, err)
			}
			union, err := resolveApplicationProviderPlanFromSnapshot(r, snapshot, preferences)
			if err != nil || len(union.ReleasedBindings) != 1 || union.ReleasedBindings[0].ProviderInstanceID != old {
				t.Fatalf("original binding lost from release/retirement: %+v %v", union, err)
			}
			requests, err := backendBindingChangeRequests(r, union, applicationProviderChangeRequests(union))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, request := range requests {
				if request.InstanceID == old {
					found = request.Selection == "explicit" && request.ReplacementID == fp.Requirements[0].InstanceID && request.Operation == "replace"
				}
			}
			if !found {
				t.Fatal("placement replacement omitted from concrete consent")
			}
			observation := observeApplicationProviderImpact(context.Background(), r, snapshot)
			if len(observation.Providers[old].Effects) < 2 || len(observation.Functions) == 0 {
				t.Fatal("retained original data and lost access consequences missing")
			}
			revision := observation.Revision
			record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
			if err != nil {
				t.Fatal(err)
			}
			record.Applied.GeneratedState["backend_placements"] = "{}"
			if err := deployment.SaveDeploymentRecord(record); err != nil {
				t.Fatal(err)
			}
			changed := observeApplicationProviderImpact(context.Background(), r, snapshot)
			if changed.Revision == revision {
				t.Fatal("placement owner change did not invalidate observed revision")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("planning replaced original authoritative binding")
			}
		})
	}
}

func TestBackendPlacementOwnerPinsOriginalAcrossPolicyChangeAndRetry(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	r = captureBackendSelectionForTest(t, r)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := recordDeploymentBeforeMutation(context.Background(), r, "applying"); err != nil {
			t.Fatal(err)
		}
		prior, err := retainedApplicationBackendIntent(r)
		if err != nil {
			t.Fatal(err)
		}
		placement, err := application.ResolveProviderPlacement(prior, capability.ProviderPostgreSQL)
		if err != nil || placement.Scope != capability.ScopeShared {
			t.Fatal("retry rewrote original realization using current operator policy", err)
		}
		owners, err := applicationRuntimeOwners(r)
		if err != nil || len(owners) != 2 {
			t.Fatalf("same intent with different placements lost an owner: %d %v", len(owners), err)
		}
		encoded, err := json.Marshal(prior)
		if err != nil || string(encoded) == "" {
			t.Fatal(err)
		}
		var portable map[string]any
		if json.Unmarshal(encoded, &portable) != nil {
			t.Fatal("invalid canonical portable encoding")
		}
		if _, leaked := portable["backendPlacements"]; leaked {
			t.Fatal("realization placement leaked into application intent")
		}
	}
	if err := recordAppliedDeployment(context.Background(), r, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil || record.Applied.GeneratedState["previous_backend_placements"] != "" {
		t.Fatal("verified convergence retained a pending placement projection", err)
	}
}

func TestRetainedBackendPlacementRejectsInvalidOwnerProjection(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	for _, invalid := range []string{"{", "null", `{"keycloak":{"scope":"shared","ownership":"baseharbor"}}`, `{"postgresql":{"scope":"application","ownership":"external"}}`} {
		if _, err := retainedBackendPlacementProjection(r, r.Manifest, invalid); err == nil {
			t.Fatal("damaged/foreign projection changed native ownership semantics")
		}
	}
}

func TestInterruptedBackendReplacementProtectsBothRealizationsFromSiblingApply(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	actual, err := (capability.RegistryStore{Path: filepath.Join(r.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	old := actual.Bindings[0].ProviderInstanceID
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	r = captureBackendSelectionForTest(t, r)
	if _, err := recordDeploymentBeforeMutation(context.Background(), r, "applying"); err != nil {
		t.Fatal(err)
	}
	if err := application.RetainPendingProviderOwnersAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	destination := planned.Bindings[0].ProviderInstanceID
	if destination == old {
		t.Fatal("fixture did not create an independent destination")
	}
	// A later sibling uses different current operator policy. The interrupted
	// deployment's existing placement owner must still protect its destination.
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	current := r
	current.Manifest = application.New("current", "dev", false, false, false)
	module := application.SharedBackendRuntimeDependency(r.Target.Name, capability.ProviderPostgreSQL)
	facts := map[string]application.ProviderFact{
		old:         {Target: r.Target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{module.ID}},
		destination: {Target: r.Target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{}},
		module.ID:   {Target: r.Target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "runtime/shared"},
	}
	snapshot, preferences, err := applicationLifecycleSnapshot(current, facts)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(current, snapshot, preferences)
	if err != nil || !plan.Complete || len(plan.ReleasedBindings) != 0 {
		t.Fatalf("pending owner became an unverified/released sibling: %+v %v", plan, err)
	}
	protected := map[string]bool{}
	for _, change := range plan.Changes {
		if change.InstanceID == old || change.InstanceID == destination || change.InstanceID == module.ID {
			if change.Action != "reuse" || len(change.Consumers) != 1 {
				t.Fatalf("sibling released original or pending destination: %+v", change)
			}
			protected[change.InstanceID] = true
		}
	}
	if len(protected) != 3 {
		t.Fatal("interrupted replacement lost an authoritative provider dependency")
	}
}

func TestNamedBackendSplitDoesNotInventSingleReplacement(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	r.Manifest = application.WithSQLInstances(r.Manifest, "default", "other")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	r = captureBackendSelectionForTest(t, r)
	snapshot, preferences, err := applicationLifecycleSnapshot(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	union, err := resolveApplicationProviderPlanFromSnapshot(r, snapshot, preferences)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := backendBindingChangeRequests(r, union, applicationProviderChangeRequests(union))
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range requests {
		if request.InstanceID == union.ReleasedBindings[0].ProviderInstanceID && request.ReplacementID != "" {
			t.Fatal("one named destination falsely replaced every source namespace")
		}
	}
}

func TestAppliedBackendSelectionSurvivesUnsetPolicyAndCapabilityDisable(t *testing.T) {
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		t.Run(string(kind), func(t *testing.T) {
			r := backendBindingFixture(t, kind)
			t.Setenv(application.ProviderScopeEnv(kind), "application")
			r = captureBackendSelectionForTest(t, r)
			if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
				t.Fatal(err)
			}
			if err := recordAppliedDeployment(context.Background(), r, application.RuntimeFiles{}); err != nil {
				t.Fatal(err)
			}
			t.Setenv(application.ProviderScopeEnv(kind), "")
			selected, err := resolveAppliedBackendSelection(r)
			if err != nil {
				t.Fatal(err)
			}
			placement, err := application.ResolveProviderPlacement(selected, kind)
			if err != nil || placement.Scope != capability.ScopeApplication {
				t.Fatal("default silently replaced previously applied provider", err)
			}
			inactive := r
			inactive.Manifest.Services.SQL, inactive.Manifest.Services.Cache = false, false
			if err := recordAppliedDeployment(context.Background(), inactive, application.RuntimeFiles{}); err != nil {
				t.Fatal(err)
			}
			if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, inactive.Manifest); err != nil {
				t.Fatal(err)
			}
			selected, err = resolveAppliedBackendSelection(r)
			if err != nil {
				t.Fatal(err)
			}
			placement, err = application.ResolveProviderPlacement(selected, kind)
			if err != nil || placement.Scope != capability.ScopeApplication {
				t.Fatal("re-enabling capability lost its retained provider selection", err)
			}
			t.Setenv(application.ProviderScopeEnv(kind), "shared")
			selected, err = resolveAppliedBackendSelection(r)
			if err != nil {
				t.Fatal(err)
			}
			placement, err = application.ResolveProviderPlacement(selected, kind)
			if err != nil || placement.Scope != capability.ScopeShared {
				t.Fatal("persisted selection permanently vetoed an explicit replacement", err)
			}
		})
	}
}

func captureBackendSelectionForTest(t *testing.T, r resolvedApplication) resolvedApplication {
	t.Helper()
	var err error
	r.Manifest, err = resolveAppliedBackendSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
