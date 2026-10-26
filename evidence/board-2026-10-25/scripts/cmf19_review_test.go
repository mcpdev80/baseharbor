package main

import (
 "context"
 "os"
 "path/filepath"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/application"
 "github.com/mcpdev80/baseharbor/internal/capability"
 "github.com/mcpdev80/baseharbor/internal/deployment"
)

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
