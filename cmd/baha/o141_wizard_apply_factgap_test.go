package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// O141 wizard/apply fact-gap reproduction.
//
// The Wizard New/Edit preview observes the application-scoped backend with a
// resolved Application whose Store.Root is empty (no deployment record exists
// yet), so its canonical runtime path resolves relative to the CWD
// ("state/<app>/runtime") and the native backend inventory ENOENTs. The Apply
// path resolves against the real deployment store root
// ("deployments/<id>/state"), where the canonical runtime inventory is
// inspectable once the first-touch Read surfaces it. This test pins that
// divergence on the disk-inspection boundary, without a runtime gate, by
// asserting nativeBackendInventory surfaces the ENOENT for the wizard store but
// not for the apply store of the same registered deployment.
func TestWizardStoreNativeBackendInventoryENOENTWhileApplyStoreInspectable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)

	app := application.New("o141-factgap", "dev", false, false, true)
	registryStore := capability.RegistryStore{Path: filepath.Join(t.TempDir(), "provider-registry.json")}
	if err := seedApplicationScopedPostgreSQLInstance(t, target, app, registryStore); err != nil {
		t.Fatalf("seed application-scoped postgresql instance: %v", err)
	}

	record := registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())
	deploymentRoot, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatalf("deployment root: %v", err)
	}

	// Wizard preview: resolved with an empty Store.Root, exactly like
	// appNewWizardAuthorityContext (no deployment record, zero identity).
	targetStateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	wizard := resolvedApplication{
		Target:          target,
		TargetStateRoot: targetStateRoot,
		Manifest:        app,
		FromRepository:  true,
	}
	// Apply path: resolved against the real deployment store root.
	apply := wizard
	apply.Store = application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name}
	apply.DeploymentIdentity = record.Identity
	apply.DeploymentRecord = &record

	if _, _, _, err := nativeBackendInventory(wizard); err == nil {
		t.Fatalf("Wizard store native backend inventory did not fail on the missing canonical runtime inventory")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Wizard store inventory failure is not the canonical runtime ENOENT: %v", err)
	}

	// Before the Apply first-touch Read materializes the canonical runtime
	// directory, the Apply store ENOENTs exactly like the Wizard store -- the
	// gap is a timing/inventory-race, not a structural Store difference.
	if _, _, _, err := nativeBackendInventory(apply); !os.IsNotExist(err) {
		t.Fatalf("Apply store without materialized runtime must ENOENT like the Wizard store (proves the race): %v", err)
	}

	// The first-touch Read in the Apply path creates the canonical runtime
	// directory before its protected compose.yaml (state/<app>/runtime/compose
	// .yaml) is supplied. Distinguish the two absences: once the directory is
	// materialized, the structural gap is closed and any remaining inventory
	// ENOENT must originate from the missing owner file, never from the
	// canonical directory itself.
	runtimeDir := filepath.Join(deploymentRoot, "state", app.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("materialize canonical runtime dir: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(runtimeDir, "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("fixture must leave only the owner compose.yaml missing: info=%v err=%v", info, err)
	}
	if _, _, _, err := nativeBackendInventory(apply); err == nil {
		t.Fatalf("Apply store with a missing owner compose.yaml must not pass the inventory")
	} else if !os.IsNotExist(err) {
		t.Fatalf("remaining inventory failure must be the missing owner file ENOENT: %v", err)
	}
	if info, err := os.Lstat(runtimeDir); err != nil || !info.IsDir() {
		t.Fatalf("canonical runtime directory must be materialized; its absence is no longer the failure cause: info=%v err=%v", info, err)
	}
}

// The named application-scoped PostgreSQL instance projects canonical
// container/volume/network resources from the registry owner without touching
// disk. This proves that, once the owner carries an application-scoped
// placement (as the wizard preview does via WithDesiredProviderPlacementsAt)
// and the named SQL instance is present, the only reason the fact stays
// "unverifiable" is the missing canonical runtime directory inspected by
// nativeBackendInventory -- not a missing instance projection.
func TestWizardNamedInstanceProjectsResourcesWithoutDisk(t *testing.T) {
	target := configureTestTarget(t)
	app := application.New("o141-factgap", "dev", true, false, false)
	app, err := application.WithBackendPlacementProjection(app, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatalf("app-scoped placement: %v", err)
	}
	if !application.HasApplicationScopedRuntimeServices(app) {
		t.Fatal("seeded app is not application-scoped")
	}
	registryStore := capability.RegistryStore{Path: filepath.Join(t.TempDir(), "provider-registry.json")}
	if err := seedApplicationScopedPostgreSQLInstance(t, target, app, registryStore); err != nil {
		t.Fatalf("seed application-scoped postgresql instance: %v", err)
	}

	record := registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())
	deploymentRoot, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatalf("deployment root: %v", err)
	}
	applyTargetStateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	apply := resolvedApplication{
		Target:             target,
		TargetStateRoot:    applyTargetStateRoot,
		Manifest:           app,
		FromRepository:     true,
		Store:              application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
		DeploymentIdentity: record.Identity,
		DeploymentRecord:   &record,
	}
	owners, err := applicationRuntimeOwners(apply)
	if err != nil {
		t.Fatalf("applicationRuntimeOwners: %v", err)
	}
	if len(owners) == 0 {
		t.Fatal("expected at least one owner for the named application-scoped instance")
	}
	var scopedOwner *resolvedApplication
	for i := range owners {
		if application.HasApplicationScopedRuntimeServices(owners[i].Manifest) {
			scopedOwner = &owners[i]
		}
	}
	if scopedOwner == nil {
		t.Fatal("no application-scoped owner projected")
	}
	intents, err := nativeBackendOwnerIntents(*scopedOwner)
	if err != nil {
		t.Fatalf("nativeBackendOwnerIntents: %v", err)
	}
	files := application.RuntimeFilesFor(scopedOwner.Store, scopedOwner.Manifest)
	union, err := nativeBackendOwnedResourceUnion(intents, files, nil)
	if err != nil || len(union) == 0 {
		t.Fatalf("named application-scoped instance projects no canonical resources: union=%v err=%v", union, err)
	}
}

// O142 close the wizard/apply inventory race: a wizard `resolved` with an empty
// Store.Root whose owner intent and a bound application-scoped baseharbor-owned
// PostgreSQL instance are provable from the registry+binding, but whose canonical
// runtime directory is missing, must resolve to ProviderStoppedOwned instead of
// dropping to ProviderUnverifiable. The fact is derived without any disk
// inspection, exactly as the wizard preview observes it.
func TestWizardStoppedOwnedFromRegistryBoundInstanceWithoutDisk(t *testing.T) {
	target := configureTestTarget(t)
	app := application.New("o142-consentfix", "dev", true, false, false)
	app, err := application.WithBackendPlacementProjection(app, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatalf("app-scoped placement: %v", err)
	}
	if !application.HasApplicationScopedRuntimeServices(app) {
		t.Fatal("seeded app is not application-scoped")
	}
	targetStateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	// The wizard preview loads the registry from TargetStateRoot, so seed it
	// there (not a scratch TempDir) for the observation to see it.
	registryStore := capability.RegistryStore{Path: filepath.Join(targetStateRoot, "provider-registry.json")}
	if err := seedApplicationScopedPostgreSQLInstance(t, target, app, registryStore); err != nil {
		t.Fatalf("seed application-scoped postgresql instance: %v", err)
	}
	instanceID := application.ApplicationProviderInstanceIDForScopedPostgreSQL(app.ApplicationID, "dev", "default")
	// The advertised condition requires an exact registry binding, so the
	// positive fixture must carry a real one (CMF19 F2), not only an instance.
	if err := bindApplicationScopedPostgreSQLInstance(t, registryStore, app, instanceID, "default"); err != nil {
		t.Fatalf("bind application-scoped postgresql instance: %v", err)
	}
	registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())

	facts := map[string]application.ProviderFact{}
	diagnostic := observeNativeBackendProviderFacts(context.Background(), resolvedApplication{
		Target:          target,
		TargetStateRoot: targetStateRoot,
		Manifest:        app,
		FromRepository:  true,
	}, facts)

	fact, observed := facts[instanceID]
	if !observed {
		t.Fatalf("wizard preview recorded no fact for the registry-bound application-scoped instance %q", instanceID)
	}
	if fact.State != application.ProviderStoppedOwned {
		t.Fatalf("wizard fact for registry-bound owned instance is %q, want ProviderStoppedOwned (o142 fix)", fact.State)
	}
	if fact.Target != target.Name {
		t.Fatalf("wizard fact target is %q, want %q", fact.Target, target.Name)
	}
	// The preview fact must be accompanied by the typed machine diagnostic so
	// it can never be consumed as verified runtime success (CMF19 F5).
	var machineErr *machine.Error
	if !errors.As(diagnostic, &machineErr) || machineErr.CauseCode != causeRuntimeInventoryNotYetMaterialized || machineErr.Resource != instanceID {
		t.Fatalf("wizard observation diagnostic is not the typed materialization error for %q: %+v", instanceID, diagnostic)
	}
}

// O142 fail-closed: a manifest with no owner intent and no bound application-scoped
// owned instance keeps the unverifiable default, so genuine inventory failures are
// never masked as stopped-but-owned.
func TestWizardUnverifiableWithoutIntentOrBoundInstance(t *testing.T) {
	target := configureTestTarget(t)
	app := application.New("o142-unverifiable", "dev", false, false, false)
	targetStateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}
	registryStore := capability.RegistryStore{Path: filepath.Join(targetStateRoot, "provider-registry.json")}
	if err := registryStore.Save(capability.NewRegistry()); err != nil {
		t.Fatalf("save empty registry: %v", err)
	}
	registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())

	facts := map[string]application.ProviderFact{}
	observeNativeBackendProviderFacts(context.Background(), resolvedApplication{
		Target:          target,
		TargetStateRoot: targetStateRoot,
		Manifest:        app,
		FromRepository:  true,
	}, facts)
	for id, fact := range facts {
		if fact.State == application.ProviderStoppedOwned {
			t.Fatalf("unexpected stopped-but-owned fact for %q without owner intent or bound instance (fail-closed must hold)", id)
		}
	}
}

// O142 the registry-bound stopped-owned fact must be derived from the
// registry+binding projection with zero disk inspection. Proving the canonical
// resource union directly (no runtime directory present) confirms the wizard path
// has a provable source before the fix records the fact.
func TestWizardRegistryBoundUnionProvableWithoutAnyDisk(t *testing.T) {
	target := configureTestTarget(t)
	app := application.New("o142-union", "dev", true, false, false)
	app, err := application.WithBackendPlacementProjection(app, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatalf("app-scoped placement: %v", err)
	}
	registryStore := capability.RegistryStore{Path: filepath.Join(t.TempDir(), "provider-registry.json")}
	if err := seedApplicationScopedPostgreSQLInstance(t, target, app, registryStore); err != nil {
		t.Fatalf("seed application-scoped postgresql instance: %v", err)
	}
	registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())

	intents, err := nativeBackendOwnerIntents(resolvedApplication{Target: target, Manifest: app, FromRepository: true})
	if err != nil {
		t.Fatalf("nativeBackendOwnerIntents: %v", err)
	}
	registry, err := (capability.RegistryStore{Path: registryStore.Path}).Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	var bound *capability.ProviderInstance
	for i := range registry.Instances {
		if applicationNativeBackendInstance(registry.Instances[i], app) {
			bound = &registry.Instances[i]
			break
		}
	}
	if bound == nil {
		t.Fatal("no bound owned application-scoped instance found")
	}
	files := application.RuntimeFilesFor(application.Store{}, app)
	union, err := nativeBackendOwnedResourceUnion(intents, files, bound)
	if err != nil || len(union) == 0 {
		t.Fatalf("registry-bound owned instance projects no canonical resources without disk: union=%v err=%v", union, err)
	}
	if bhruntime.ContainerMatchesProjectResource("docker", bhruntime.RuntimeContainer{}, "", bhruntime.ProjectResource{Kind: "container", Name: ""}) {
		t.Fatalf("expected no container match for an empty runtime resource")
	}
}

// bindApplicationScopedPostgreSQLInstance creates the real registry binding for
// a named, application-scoped PostgreSQL instance of this manifest consumer.
func bindApplicationScopedPostgreSQLInstance(t *testing.T, store capability.RegistryStore, app application.Manifest, instanceID, resourceName string) error {
	t.Helper()
	registry, err := store.Load()
	if err != nil {
		return err
	}
	if err := registry.BindDeployment(capability.Resource{Application: app.Name, Kind: capability.SQL, Name: resourceName, Provider: capability.ProviderPostgreSQL}, app.ApplicationID, app.Environment, instanceID); err != nil {
		return err
	}
	return store.Save(registry)
}

func seedApplicationScopedPostgreSQLInstance(t *testing.T, target deployment.ResolvedTarget, app application.Manifest, store capability.RegistryStore) error {
	t.Helper()
	instanceID := application.ApplicationProviderInstanceIDForScopedPostgreSQL(app.ApplicationID, "dev", "default")
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{
		ID:                 instanceID,
		Provider:           capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Scope:              capability.ScopeApplication,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: app.ApplicationID,
		OwnerApplication:   app.Name,
		OwnerEnvironment:   "dev",
	}); err != nil {
		return err
	}
	return store.Save(registry)
}
