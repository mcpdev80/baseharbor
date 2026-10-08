// Package providerbinding connects reviewed provider adapters to the existing
// Core runtime and recovery seams. It does not schedule updates or own SQL migrations.
package providerbinding

import (
 "context"
 "errors"
 "fmt"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/identityprovider"
 nativebao "github.com/mcpdev80/baseharbor/internal/openbao"
 "github.com/mcpdev80/baseharbor/internal/providerupgrade"
 "github.com/mcpdev80/baseharbor/internal/providerupgrade/keycloak"
 "github.com/mcpdev80/baseharbor/internal/providerupgrade/openbao"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeIdentity is evidence obtained from the selected RuntimeProvider,
// not from a user-supplied provider name. The Engine/project association must
// be verified by the runtime implementation before marking a service owned.
type RuntimeIdentity struct {
 Provider providerupgrade.Provider
 Project string
 Service string
 Engine string
 Image string
 Digest string
 Owned bool
}

// RuntimeInventory must consult runtime resource labels and observed image
// identities; external or missing provider realizations fail closed.
type RuntimeInventory interface {
 InspectManaged(context.Context, providerupgrade.Provider) (RuntimeIdentity, error)
}

// Dependencies are supplied by the existing Core installation/update engine.
// Snapshot, restore and pinned reconciliation always remain Core-owned.
type Dependencies struct {
 Inventory RuntimeInventory
 OpenBaoExecutor nativebao.Executor
 OpenBaoFiles bhruntime.Files
 OpenBaoHooks openbao.RuntimeHooks
 KeycloakDataDir string
 KeycloakNamespace string
 KeycloakInstallationID string
 KeycloakIssuer string
 KeycloakHooks keycloak.CoreHooks
}

// Registry exposes only known providers and does not perform any mutations.
type Registry struct {
 providers map[providerupgrade.Provider]providerupgrade.Adapter
 inventory RuntimeInventory
}

func New(deps Dependencies) (*Registry,error) {
 if deps.Inventory==nil { return nil, errors.New("Core RuntimeProvider inventory is required") }
 if deps.OpenBaoExecutor==nil || deps.OpenBaoFiles.Project=="" || deps.OpenBaoFiles.Compose=="" || deps.OpenBaoFiles.Env=="" {
  return nil,errors.New("owned OpenBao executor and existing Core runtime files are required")
 }
 if deps.KeycloakDataDir=="" || deps.KeycloakInstallationID=="" || deps.KeycloakIssuer=="" {
  return nil,errors.New("existing Keycloak Core data directory, installation identity and issuer are required")
 }
 if _,err:=identityprovider.ExistingCoreRuntimeFiles(deps.KeycloakDataDir,deps.KeycloakNamespace);err!=nil {
  return nil,fmt.Errorf("existing Keycloak Core realization not found: %w",err)
 }
 if err:=requireHooks(deps);err!=nil {return nil,err}
 return &Registry{
  providers:map[providerupgrade.Provider]providerupgrade.Adapter{
   providerupgrade.ProviderOpenBao:openbao.New(&openbao.NativeOps{Executor:deps.OpenBaoExecutor,Files:deps.OpenBaoFiles,Owner:"baseharbor",Hooks:deps.OpenBaoHooks}),
   providerupgrade.ProviderKeycloak:keycloak.New(&keycloak.NativeOps{DataDir:deps.KeycloakDataDir,Namespace:deps.KeycloakNamespace,InstallationID:deps.KeycloakInstallationID,ExpectedIssuer:deps.KeycloakIssuer,Hooks:deps.KeycloakHooks}),
  },
  inventory:deps.Inventory,
 },nil
}

func requireHooks(d Dependencies) error {
 b:=d.OpenBaoHooks
 if b.UpgradePath==nil||b.Backup==nil||b.VerifyBackup==nil||b.Apply==nil||b.Unseal==nil||b.VerifyAuth==nil||b.VerifyApps==nil||b.Restore==nil {
  return providerupgrade.Wrap(providerupgrade.ErrorDependency,"openbao bindings",errors.New("Core backup, pinned reconcile, auth, application access and recovery hooks are required"))
 }
 k:=d.KeycloakHooks
 if k.Inspect==nil||k.Compatibility==nil||k.Backup==nil||k.VerifyBackup==nil||k.ApplySingle==nil||k.StopHA==nil||k.ApplyHA==nil||k.ReplaceMember==nil||k.WaitMember==nil||k.WaitAll==nil||k.VerifySQL==nil||k.VerifyRealms==nil||k.VerifyTokens==nil||k.Restore==nil {
  return providerupgrade.Wrap(providerupgrade.ErrorDependency,"keycloak bindings",errors.New("Core SQL snapshot, topology, readiness, realm, token and recovery hooks are required"))
 }
 return nil
}

func (r *Registry) Resolve(ctx context.Context, provider providerupgrade.Provider) (providerupgrade.Adapter,RuntimeIdentity,error) {
 if r==nil || r.inventory==nil {return nil,RuntimeIdentity{},providerupgrade.Wrap(providerupgrade.ErrorInvalidState,"registry",errors.New("uninitialized registry"))}
 adapter,ok:=r.providers[provider]
 if !ok {return nil,RuntimeIdentity{},providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath,"registry",errors.New("unsupported provider"))}
 identity,err:=r.inventory.InspectManaged(ctx,provider)
 if err!=nil{return nil,RuntimeIdentity{},providerupgrade.Wrap(providerupgrade.ErrorInvalidState,"runtime inventory",err)}
 if identity.Provider!=provider||!identity.Owned||strings.TrimSpace(identity.Project)==""||strings.TrimSpace(identity.Service)==""||strings.TrimSpace(identity.Image)==""||!strings.HasPrefix(identity.Digest,"sha256:")||len(identity.Digest)!=71 {
  return nil,RuntimeIdentity{},providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath,"runtime ownership",errors.New("missing or foreign runtime provider identity, project, service or immutable digest"))
 }
 inventory,err:=adapter.Inventory(ctx)
 if err!=nil{return nil,RuntimeIdentity{},err}
 if inventory.Provider!=provider||inventory.Owner!="baseharbor" {
  return nil,RuntimeIdentity{},providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath,"adapter ownership",errors.New("provider inventory is foreign or inconsistent"))
 }
 return adapter,identity,nil
}

// Preflight is the sole plan preparation entrypoint exposed to Session 1A.
// Every mutation must use the adapter returned by Resolve and the durable
// central journal; this package must never invoke Execute independently.
func (r *Registry) Preflight(ctx context.Context,provider providerupgrade.Provider,request providerupgrade.Request)(providerupgrade.Adapter,providerupgrade.Assessment,error){
 adapter,_,err:=r.Resolve(ctx,provider)
 if err!=nil{return nil,providerupgrade.Assessment{},err}
 assessment,err:=adapter.Preflight(ctx,request)
 if err!=nil{return nil,providerupgrade.Assessment{},err}
 return adapter,assessment,nil
}
