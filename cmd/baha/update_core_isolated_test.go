package main

import (
 "context"
 "encoding/json"
 "errors"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/application"
 "github.com/mcpdev80/baseharbor/internal/coreupdate"
 "github.com/mcpdev80/baseharbor/internal/deployment"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type isolatedInventoryRuntime struct {
 bhruntime.RuntimeProvider
 Containers []bhruntime.RuntimeContainer
 Images map[string]bhruntime.ImageIdentity
}
func (r isolatedInventoryRuntime) ListRuntimeContainers(context.Context)([]bhruntime.RuntimeContainer,error){return r.Containers,nil}
func (r isolatedInventoryRuntime) ProjectServiceImageIdentity(_ context.Context,project,service string)(bhruntime.ImageIdentity,error){
 item,ok:=r.Images[project+"/"+service]
 if !ok{return bhruntime.ImageIdentity{},errors.New("missing image")}
 return item,nil
}

func TestIsolatedCoreInventoryOnlyRegisteredOwnedProject(t *testing.T){
 t.Setenv("XDG_DATA_HOME",t.TempDir())
 m:=application.New("app","dev",true,false,false)
 target:="isolated-inventory"
 record:=deployment.DeploymentRecord{
  Version:deployment.DeploymentRecordVersion,
  Identity:deployment.DeploymentIdentity{DeploymentID:testDeploymentID,ApplicationID:m.ApplicationID,Target:target,Application:m.Name,Environment:m.Environment},
  Applied:deployment.AppliedDeployment{RuntimeProvider:"docker"},
 }
 record.Applied.Intent,_=json.Marshal(m)
 if err:=deployment.SaveDeploymentRecord(record);err!=nil{t.Fatal(err)}
 files:=application.RuntimeFilesFor(application.Store{Namespace:target},m)
 catalog,err:=coreupdate.LoadRelease("0.4.24")
 if err!=nil{t.Fatal(err)}
 img:=bhruntime.ImageIdentity{Reference:"docker.io/library/postgres:18-alpine",Digest:catalog.Providers[0].Digest}
 mock:=isolatedInventoryRuntime{
  Containers:[]bhruntime.RuntimeContainer{
   {Project:files.Project,Service:"postgres",Running:true},
   {Project:"foreign-project",Service:"postgres",Running:true},
   {Project:files.Project,Service:"app",Running:true},
  },
  Images:map[string]bhruntime.ImageIdentity{files.Project+"/postgres":img},
 }
 deltas,err:=inspectIsolatedCoreProviders(context.Background(),mock,target,catalog)
 if err!=nil{t.Fatal(err)}
 if len(deltas)!=1{t.Fatalf("expected one isolated owned SQL, got %+v",deltas)}
 if deltas[0].Installed.Scope!="application" || deltas[0].Installed.Owner!="baseharbor" || deltas[0].Installed.Installation!=record.Identity.DeploymentID{
  t.Fatalf("wrong isolated ownership %+v",deltas[0])
 }
 if deltas[0].Classification!=coreupdate.NoChange{t.Fatalf("identical pinned image needs no restart: %+v",deltas[0])}
 mock.Containers[0].Running=false
 if _,err:=inspectIsolatedCoreProviders(context.Background(),mock,target,catalog);err==nil{t.Fatal("stopped owned provider was ignored")}
}

func TestIsolatedCoreServiceClassifierExcludesAccessProxies(t *testing.T){
 for _,name:=range []string{"postgres-access","keycloak-access","openbao-ui","postgres-exporter","keycloak-admin"} {
  if _,ok:=isolatedCoreServiceKind(name);ok{t.Fatalf("gateway or proxy %s misclassified as Core",name)}
 }
}
