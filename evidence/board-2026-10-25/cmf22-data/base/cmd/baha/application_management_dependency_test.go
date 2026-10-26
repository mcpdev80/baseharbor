package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func managementDependencyFixture(t *testing.T) (resolvedApplication, map[string]application.ProviderFact) {
	t.Helper()
	files := coreStopFixture(t)
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: application.WithWorkloadComponents(application.New("plain", "dev", false, false, false), "api")}
	spec := coreinstallation.Spec{Target: target.Name, Runtime: target.RuntimeProvider, MachineRole: coreinstallation.Development}
	state := coreinstallation.State{Version: "baseharbor.core-installation/v1", ID: application.MustNewApplicationID(), Owner: "baseharbor", Spec: spec, Defaults: spec.MachineRole.Defaults(), Placement: "shared", Phase: "preflight", Capabilities: map[string]bool{"sql": false, "secrets": false, "identity": false}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(files.Compose), "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	identity, owner, err := identityprovider.CoreIdentityDependency(root, target.Name)
	if err != nil {
		t.Fatal(err)
	}
	storage := application.RuntimeStorageDependency(target.Name)
	facts := map[string]application.ProviderFact{}
	for _, item := range []struct {
		id, owner string
		deps      []string
	}{{identity.ID, owner, []string{storage.ID}}, {managementSecretsDependency(target.Name).ID, filepath.Dir(files.Compose), []string{storage.ID}}, {storage.ID, filepath.Dir(files.Compose), []string{}}} {
		facts[item.id] = application.ProviderFact{Target: target.Name, State: application.ProviderRunning, Core: application.CoreRequired, Ancillary: true, OwnerReference: item.owner, Dependencies: item.deps}
	}
	return resolved, facts
}

func TestIndependentManagementHasPhysicalDependenciesWithoutAppRegistryOffers(t *testing.T) {
	resolved, facts := managementDependencyFixture(t)
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "installation.json")
	before, _ := os.ReadFile(path)
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, facts)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil || !plan.Complete {
		t.Fatalf("independent owner union: %+v %v", plan, err)
	}
	if len(plan.Required) != 3 || len(snapshot.Registry.Instances) != 0 || len(snapshot.Registry.Bindings) != 0 {
		t.Fatalf("missing native roots or invented selectable offers: %+v %+v", plan.Required, snapshot.Registry)
	}
	for _, change := range plan.Changes {
		if change.Action != "reuse" || len(change.Consumers) != 1 {
			t.Fatalf("management runtime retired: %+v", change)
		}
	}
	if footprint := plan.Consumers["application/"+resolved.Manifest.ApplicationID+"/dev"]; footprint.Core != application.CoreNotRequired {
		t.Fatalf("independent management forced Core on plain app: %+v", footprint)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("read mutated installation owner")
	}
	if _, err := os.Lstat(filepath.Join(resolved.TargetStateRoot, "provider-registry.json")); !os.IsNotExist(err) {
		t.Fatal("read allocated application registry")
	}
}

func TestManagementProtectsPhysicalSharedIdentityAliasesButNotIndependentProviders(t *testing.T) {
	resolved, _ := managementDependencyFixture(t)
	registry := capability.NewRegistry()
	registry.Instances = []capability.ProviderInstance{
		{ID: "shared-identity", Provider: capability.Keycloak, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "team-a"},
		{ID: "application-identity", Provider: capability.Keycloak, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		{ID: "external-identity", Provider: capability.Keycloak, Scope: capability.ScopeShared, Ownership: capability.OwnershipExternal},
		{ID: "canonical-secrets", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "separate-secrets", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "another-module"},
	}
	roots, err := managementProviderDependencyRoots(resolved, application.ResolutionSnapshot{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"shared-identity", "canonical-secrets"} {
		if !slices.Contains(roots, id) {
			t.Fatalf("physical alias unprotected: %s", id)
		}
	}
	for _, id := range []string{"application-identity", "external-identity", "separate-secrets"} {
		if slices.Contains(roots, id) {
			t.Fatalf("unrelated provider became management root: %s", id)
		}
	}
}

func TestManagementUnknownNativeDependencyIsPreservedNotDeclaredRunning(t *testing.T) {
	resolved, _ := managementDependencyFixture(t)
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil || plan.Complete {
		t.Fatalf("unknown native owner became complete: %+v %v", plan, err)
	}
	for _, change := range plan.Changes {
		if change.Action != "preserve" {
			t.Fatalf("unknown native owner mutated: %+v", change)
		}
	}
}

func TestPlainApplicationWithoutManagementAllocatesNoInstallationDependencies(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	snapshot, _, err := applicationLifecycleSnapshot(resolved, nil)
	if err != nil || len(snapshot.Ancillary) != 0 {
		t.Fatalf("implicit management: %+v %v", snapshot.Ancillary, err)
	}
	if _, err := os.Lstat(resolved.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("plain read wrote Target state")
	}
}
