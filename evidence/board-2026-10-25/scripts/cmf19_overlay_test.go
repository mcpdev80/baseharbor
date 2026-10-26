package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
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
	registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())

	facts := map[string]application.ProviderFact{}
	observeNativeBackendProviderFacts(context.Background(), resolvedApplication{
		Target:          target,
		TargetStateRoot: targetStateRoot,
		Manifest:        app,
		FromRepository:  true,
	}, facts)

	instanceID := application.ApplicationProviderInstanceIDForScopedPostgreSQL(app.ApplicationID, "dev", "default")
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

func cmf19Fixture(t *testing.T) resolvedApplication {
 t.Helper()
 target := configureTestTarget(t)
 app := application.New("cmf19-neutral", "dev", true, false, false)
 app, err := application.WithBackendPlacementProjection(app, map[capability.ProviderKind]capability.ProviderPlacement{
  capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
 })
 if err != nil { t.Fatal(err) }
 root, err := deployment.TargetStateRoot(target.Name)
 if err != nil { t.Fatal(err) }
 if err := seedApplicationScopedPostgreSQLInstance(t, target, app, capability.RegistryStore{Path:filepath.Join(root,"provider-registry.json")}); err != nil { t.Fatal(err) }
 registerTestDeployment(t, target, app, t.TempDir(), t.TempDir())
 return resolvedApplication{Target:target, TargetStateRoot:root, Manifest:app, FromRepository:true, Store:application.Store{Root:filepath.Join(t.TempDir(),"state"),Namespace:target.Name}}
}

func TestCMF19UnboundInstanceMustRemainUnverifiable(t *testing.T) {
 resolved := cmf19Fixture(t)
 resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
 registry, err := (capability.RegistryStore{Path:filepath.Join(resolved.TargetStateRoot,"provider-registry.json")}).Load()
 if err != nil {t.Fatal(err)}
 if len(registry.Bindings) != 0 {t.Fatal("fixture must be unbound")}
 facts := map[string]application.ProviderFact{}
 observeNativeBackendProviderFacts(context.Background(), resolved, facts)
 if len(facts) != 0 {t.Errorf("unbound registry instance produced verified facts: %+v", facts)}
 if err := checkRetainedApplicationProviderOwnership(context.Background(),resolved); err == nil {t.Error("unbound retained ownership admitted without runtime verification")}
}

func TestCMF19MaterializedDirectoryMissingEnvMustNotProveAbsence(t *testing.T) {
 resolved := cmf19Fixture(t)
 cmf19BindAll(t,resolved)
 files := application.RuntimeFilesFor(resolved.Store,resolved.Manifest)
 if err := os.MkdirAll(files.Dir,0700); err != nil {t.Fatal(err)}
 compose, err := application.RuntimeComposeYAMLForProject(resolved.Manifest,files.ResourceProject)
 if err != nil {t.Fatal(err)}
 if err := os.WriteFile(files.Compose,[]byte(compose),0600); err != nil {t.Fatal(err)}
 // runtime.env is missing: the runtime directory already exists.
 if _,_,_,err := nativeBackendInventory(resolved); !os.IsNotExist(err) {t.Fatalf("expected missing env, got %v",err)}
 resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
 facts := map[string]application.ProviderFact{}
 observeNativeBackendProviderFacts(context.Background(),resolved,facts)
 if len(facts) != 0 {t.Fatalf("missing env misclassified as missing directory and measured absence: %+v",facts)}
}

func TestCMF19TwoInstancesMustBothReceiveFacts(t *testing.T) {
 resolved := cmf19Fixture(t)
 resolved.Manifest = application.WithSQLInstances(resolved.Manifest,"default","analytics")
 store := capability.RegistryStore{Path:filepath.Join(resolved.TargetStateRoot,"provider-registry.json")}
 registry,err := store.Load()
 if err != nil {t.Fatal(err)}
 second := registry.Instances[0]
 second.ID = application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID,"dev","analytics")
 if err := registry.Register(second); err != nil {t.Fatal(err)}
 if err := store.Save(registry); err != nil {t.Fatal(err)}
 cmf19BindAll(t,resolved)
 facts := map[string]application.ProviderFact{}
 observeNativeBackendProviderFacts(context.Background(),resolved,facts)
 if len(facts) != 2 {t.Fatalf("only %d/2 projected instances observed: %+v",len(facts),facts)}
}

func cmf19BindAll(t *testing.T, resolved resolvedApplication) {
 t.Helper()
 store := capability.RegistryStore{Path:filepath.Join(resolved.TargetStateRoot,"provider-registry.json")}
 registry,err := store.Load()
 if err != nil {t.Fatal(err)}
 for _,instance := range registry.Instances {
  name := "default"
  if instance.ID == application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID,"dev","analytics") {name="analytics"}
  if err := registry.BindDeployment(capability.Resource{Application:resolved.Manifest.Name,Kind:capability.SQL,Name:name,Provider:capability.ProviderPostgreSQL},resolved.Manifest.ApplicationID,"dev",instance.ID); err != nil {t.Fatal(err)}
 }
 if err := store.Save(registry); err != nil {t.Fatal(err)}
}

func TestCMF19RuntimeDetectionFailureMustRemainUnverifiable(t *testing.T) {
 resolved := cmf19Fixture(t)
 cmf19BindAll(t,resolved)
 files := application.RuntimeFilesFor(resolved.Store,resolved.Manifest)
 if err := os.MkdirAll(files.Dir,0700); err != nil {t.Fatal(err)}
 compose,err := application.RuntimeComposeYAMLForProject(resolved.Manifest,files.ResourceProject)
 if err != nil {t.Fatal(err)}
 if err := os.WriteFile(files.Compose,[]byte(compose),0600); err != nil {t.Fatal(err)}
 if err := os.WriteFile(files.Env,[]byte(""),0600); err != nil {t.Fatal(err)}
 if _,_,_,err := nativeBackendInventory(resolved); err != nil {t.Fatalf("fixture inventory must be valid: %v",err)}
 resolved.Target.RuntimeProvider = "cmf19-unsupported-runtime"
 if _,err := detectRuntimeForTarget(context.Background(),resolved.Target); err == nil {t.Fatal("fixture must fail detection")}
 facts := map[string]application.ProviderFact{}
 observeNativeBackendProviderFacts(context.Background(),resolved,facts)
 if len(facts) != 0 {t.Fatalf("runtime detection failure converted into measured absence: %+v",facts)}
}
