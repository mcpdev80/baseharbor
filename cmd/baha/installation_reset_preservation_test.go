package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestInstallationResetPreservesSharedForeignProvider is the permanent
// regression for the shared/foreign provider PRESERVATION ASSERTION behind
// card 0428-287 / PR #1835.
//
// It proves that during installation reset / failed-apply destroy, providers
// that are NOT owned by the reset target (shared and foreign, including the
// Core-owned shared SQL backend already modeled in cmd/baha/app_core_requirement
// and provider_registry) keep their state and are not re-planned for removal:
//
//   - a Core-owned shared PostgreSQL instance is registered on the Target
//     alongside the selected app's own application-scoped providers,
//   - the selected app is left in a stuck/failed-apply state (a persisted
//     deployment record with applied intent + last_applied_ref, state "applying"
//     but no runtime directory ever materialized),
//   - the materialized destroy plan must route through the incomplete-inventory
//     recovery: plan.Providers.Impact stays nil so the shared provider is never
//     added to the acknowledged consequence plan, and
//   - the real CLI full-reset destroy with the two current-plan confirmations
//     plus separate data consent returns rc=0 while the shared provider's
//     registry instance and binding survive the destroy.
//
// On the current baseline the same state fails closed with "retained provider
// ownership could not be verified through its canonical deployment and native
// renderer" because the Core-owned PostgreSQL instance cannot be observed
// through the application's own absent runtime, so the whole install reset is
// aborted instead of preserving the unrelated shared provider.
func TestInstallationResetPreservesSharedForeignProvider(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}

	// Register the selected app's own application-scoped providers so the
	// union/retirement model has a complete app-owned surface.
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m); err != nil {
		t.Fatalf("reconcile application provider registry: %v", err)
	}

	// Register a Core-owned shared PostgreSQL provider instance that belongs to
	// the Target, not to the selected app: it is a shared, foreign-to-target
	// provider that must survive an install reset of a DIFFERENT application.
	registryPath := resolved.TargetStateRoot + "/provider-registry.json"
	registry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		t.Fatalf("load provider registry for shared provider: %v", err)
	}
	sharedSQL := capability.ProviderInstance{
		ID:              "shared-sql-" + t.Name(),
		Provider:        capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Scope:           capability.ScopeShared,
		Ownership:       capability.OwnershipBaseHarbor,
		SharingBoundary: "team:shared",
	}
	if err := registry.Register(sharedSQL); err != nil {
		t.Fatalf("register shared foreign provider: %v", err)
	}
	sharedSQLOwner := application.MustNewApplicationID()
	if err := registry.BindDeployment(capability.Resource{Kind: capability.SQL, Name: "primary", Provider: capability.ProviderPostgreSQL}, sharedSQLOwner, "", sharedSQL.ID); err != nil {
		t.Fatalf("bind shared foreign provider: %v", err)
	}
	if err := (capability.RegistryStore{Path: registryPath}).Save(registry); err != nil {
		t.Fatalf("save provider registry with shared foreign provider: %v", err)
	}

	// Reconstruct the stuck/failed-apply state exactly as a real aborted apply
	// leaves it: deployment.json present with applied intent + last_applied_ref,
	// state "applying", but NO runtime directory materialized.
	manifestBytes, err := json.Marshal(resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	pending := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: resolved.repositoryRoot(),
			Manifest:   resolved.ManifestPath,
			Digest:     "abort-revision-preservation",
		},
		Applied: deployment.AppliedDeployment{
			Intent:          json.RawMessage(manifestBytes),
			RuntimeProvider: resolved.Target.RuntimeProvider,
			LastAppliedRef:  "abort-revision-preservation",
			GeneratedState: map[string]string{
				"deployment": resolved.DeploymentStateRoot,
				"state":      resolved.DeploymentStateRoot + "/state",
			},
		},
		Observed: deployment.ObservedDeployment{State: "applying", Ready: false},
	}
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatalf("save stuck-apply record: %v", err)
	}

	// The runtime directory must be genuinely absent: this is the exact
	// ownership evidence that distinguishes "applied but never materialized"
	// from a healthy delivery.
	rtDir := application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Dir
	if _, statErr := os.Stat(rtDir); statErr == nil {
		t.Fatalf("runtime dir unexpectedly present at %s; the abort scenario requires it absent", rtDir)
	}

	// The materialized destroy plan for the stuck/failed-apply state must route
	// through the incomplete-inventory recovery: the shared/foreign provider is
	// NOT added to the acknowledged consequence plan, so it keeps its state and
	// is not re-planned for removal.
	plan, err := buildApplicationDestructionPlan(testOperatorContext(context.Background()), resolved, true)
	if err != nil {
		t.Fatalf("build destroy plan with shared foreign provider present: %v", err)
	}
	// buildIncompleteInventoryDestructionPlan never sets Providers, so
	// plan.Providers.Impact is nil by construction whenever IncompleteInventory
	// is true; the IncompleteInventory route below is therefore the sole
	// red/green differentiator. The Impact check was degenerate (tautological)
	// and removed so the guard is a clean red/green assertion.
	if !plan.IncompleteInventory {
		t.Fatal("failed-apply destroy must route through the incomplete-inventory recovery to preserve shared/foreign providers")
	}

	// The shared foreign provider registry instance survives planning.
	plannedRegistry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		t.Fatalf("reload registry after planning: %v", err)
	}
	if !registryInstancePresent(plannedRegistry, sharedSQL.ID) {
		t.Fatal("shared foreign provider instance vanished from the registry during planning")
	}

	// Two current-plan confirmations plus separate data consent are the strongest
	// acknowledgement the incomplete-inventory recovery requires.
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}
	destroyCtx := withProviderChangeConsent(testOperatorContext(context.Background()), ack)
	var out bytes.Buffer
	derr := runWithIOAsTestOperator(destroyCtx, []string{"app", "destroy", "--yes", "--full-reset", "--json"}, &out, &out)
	if derr != nil {
		t.Fatalf("failed-apply destroy must return rc=0 with a shared foreign provider present, got: %v\noutput:\n%s", derr, out.String())
	}

	// The shared foreign provider registry instance AND its binding survive the
	// install reset of the selected app.
	afterRegistry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		t.Fatalf("reload registry after destroy: %v", err)
	}
	if !registryInstancePresent(afterRegistry, sharedSQL.ID) {
		t.Fatal("shared foreign provider instance was removed by the install reset; it is not owned by the reset target")
	}
	if !registryBindingPresent(afterRegistry, sharedSQL.ID) {
		t.Fatal("shared foreign provider binding was removed by the install reset; it is not owned by the reset target")
	}

	// The selected app's deployment owner is gone while the authored source and
	// the unrelated shared provider remain.
	if _, statErr := os.Stat(resolved.DeploymentStateRoot + "/deployment.json"); !os.IsNotExist(statErr) {
		t.Fatalf("failed-apply destroy retained the selected deployment owner: %v", statErr)
	}
	if data, readErr := os.ReadFile(application.RepositoryManifestName); readErr != nil || len(data) == 0 {
		t.Fatalf("failed-apply destroy removed authored source: readErr=%v", readErr)
	}
	t.Log("install reset of failed-apply app preserved the shared/foreign provider: instance and binding retained, selected owner removed, shared provider not re-planned")
}

func registryInstancePresent(registry capability.Registry, id string) bool {
	for _, instance := range registry.Instances {
		if instance.ID == id {
			return true
		}
	}
	return false
}

func registryBindingPresent(registry capability.Registry, instanceID string) bool {
	for _, binding := range registry.Bindings {
		if binding.ProviderInstanceID == instanceID {
			return true
		}
	}
	return false
}
