package providerbinding

import (
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/providerupgrade"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type fakeReader struct{
 containers []bhruntime.RuntimeContainer
 identity bhruntime.ImageIdentity
 err error
 calls int
}
func(f *fakeReader) ListRuntimeContainers(context.Context)([]bhruntime.RuntimeContainer,error){f.calls++;return f.containers,f.err}
func(f *fakeReader) ProjectServiceImageIdentity(context.Context,string,string)(bhruntime.ImageIdentity,error){f.calls++;return f.identity,f.err}

func TestRuntimeBindingManagedIdentity(t *testing.T){
 good:=&fakeReader{containers:[]bhruntime.RuntimeContainer{{Project:"core-owned",Service:"openbao",Running:true,Health:"healthy"}},identity:bhruntime.ImageIdentity{Reference:"quay.io/openbao/openbao:2.4.4",Digest:"sha256:"+strings.Repeat("a",64)}}
 binding:=&RuntimeBinding{Reader:good,Engine:"podman",Sources:map[providerupgrade.Provider]ManagedSource{providerupgrade.ProviderOpenBao:{Project:"core-owned",Service:"openbao"}}}
 got,err:=binding.InspectManaged(context.Background(),providerupgrade.ProviderOpenBao)
 if err!=nil||!got.Owned||got.Project!="core-owned"||got.Engine!="podman"||got.Digest!=good.identity.Digest {t.Fatalf("owned inventory mismatch: %+v %v",got,err)}
}
func TestRuntimeBindingForeignMissingUnhealthyAndUnpinned(t *testing.T){
 p:=providerupgrade.ProviderOpenBao
 cases:=[]struct{name string;containers []bhruntime.RuntimeContainer;identity bhruntime.ImageIdentity}{
  {"foreign",[]bhruntime.RuntimeContainer{{Project:"unowned",Service:"openbao",Running:true}},bhruntime.ImageIdentity{}},
  {"missing",nil,bhruntime.ImageIdentity{}},
  {"stopped",[]bhruntime.RuntimeContainer{{Project:"core-owned",Service:"openbao",Running:false}},bhruntime.ImageIdentity{}},
  {"unhealthy",[]bhruntime.RuntimeContainer{{Project:"core-owned",Service:"openbao",Running:true,Health:"unhealthy"}},bhruntime.ImageIdentity{}},
  {"unpinned",[]bhruntime.RuntimeContainer{{Project:"core-owned",Service:"openbao",Running:true}},bhruntime.ImageIdentity{Reference:"openbao:latest"}},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  r:=&fakeReader{containers:tc.containers,identity:tc.identity}
  b:=&RuntimeBinding{Reader:r,Sources:map[providerupgrade.Provider]ManagedSource{p:{Project:"core-owned",Service:"openbao"}}}
  _,err:=b.InspectManaged(context.Background(),p)
  if err==nil {t.Fatal("foreign or unverifiable provider accepted")}
 })}
}
func TestRegistryRejectsMissingSQLAndRecoveryBindings(t *testing.T){
 _,err:=New(Dependencies{})
 if err==nil {t.Fatal("registry accepted absent runtime")}
 if !errors.Is(providerupgrade.Wrap(providerupgrade.ErrorDependency,"test",errors.New("missing")),errors.New("missing")) {
  // ClassOf, not errors.Is against a newly allocated sentinel, is the stable contract.
  if providerupgrade.ClassOf(providerupgrade.Wrap(providerupgrade.ErrorDependency,"test",errors.New("missing")))!=providerupgrade.ErrorDependency {t.Fatal("error class lost")}
 }
}
