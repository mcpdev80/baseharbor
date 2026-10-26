
func TestCMF20RegistryProjectionCannotVerifyRuntimeRetirement(t *testing.T) {
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
 observed:=observeApplicationProviderFacts(context.Background(),resolved)
 plan:=application.ProviderPlan{Version:application.ProviderPlanVersion,Target:target.Name,Complete:true,Changes:[]application.ProviderChange{{InstanceID:instanceID,Ownership:capability.OwnershipBaseHarbor,Action:"stop"}}}
 if err:=verifyApplicationRuntimeRetirement(resolved,plan,observed);err==nil {t.Fatalf("unmeasured registry-only projection authorized actual retirement verification: %+v",observed[instanceID])}
}
