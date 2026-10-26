package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
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
	// directory with its protected compose.yaml (state/<app>/runtime/compose
	// .yaml). Once present, the same inventory is inspectable and the fact is
	// no longer structurally unverifiable.
	if err := os.MkdirAll(filepath.Join(deploymentRoot, "state", app.Name, "runtime"), 0o700); err != nil {
		t.Fatalf("materialize canonical runtime dir: %v", err)
	}
	if _, _, _, err := nativeBackendInventory(apply); !os.IsNotExist(err) {
		// CheckExistingNativeBackendInventory still fails (missing compose.yaml),
		// but it no longer ENOENTs on the directory -- the structural gap is
		// closed; only the file content remains to be supplied by the Read.
		t.Fatalf("Apply store with materialized runtime dir must not ENOENT on the directory (only a missing compose.yaml is allowed): %v", err)
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
