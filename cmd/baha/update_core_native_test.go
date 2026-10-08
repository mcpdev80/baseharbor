package main
import(
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/coreupdate"
 "github.com/mcpdev80/baseharbor/internal/identityprovider"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)
type fakeNativeCoreRuntime struct{
 bhruntime.RuntimeProvider
 containers []bhruntime.RuntimeContainer
 image bhruntime.ImageIdentity
 stopped,started int
}
func(f *fakeNativeCoreRuntime)StopProject(_ context.Context,_,_,_ string)error{
 f.stopped++
 return nil
}
func(f *fakeNativeCoreRuntime)UpProject(_ context.Context,_,_,_ string)error{
 f.started++
 return nil
}
func(f *fakeNativeCoreRuntime)ListRuntimeContainers(context.Context)([]bhruntime.RuntimeContainer,error){return f.containers,nil}
func(f *fakeNativeCoreRuntime)ProjectServiceImageIdentity(context.Context,string,string)(bhruntime.ImageIdentity,error){return f.image,nil}

func TestNativeCoreQuiesceRefusesActiveWriters(t *testing.T){
 runtime:=&fakeNativeCoreRuntime{containers:[]bhruntime.RuntimeContainer{{Project:"owned-core",Service:"postgres-member-1",Running:true}}}
 ops:=&coreNativeRuntimeOps{runtime:runtime,core:bhruntime.Files{Project:"owned-core",Compose:"core.yaml",Env:"core.env"}}
 delta:=coreupdate.Delta{Installed:coreupdate.Realization{Kind:coreupdate.SQL,Instance:"postgres-member-1"}}
 if err:=ops.Quiesce(context.Background(),delta);err==nil{t.Fatal("active writer treated as quiesced")}
 if runtime.stopped!=1{t.Fatal("Core stop not attempted")}
 runtime.containers[0].Running=false
 if err:=ops.Quiesce(context.Background(),delta);err!=nil{t.Fatal(err)}
}
func TestNativeCorePinnedImageVerificationRefusesDigestDrift(t *testing.T){
 runtime:=&fakeNativeCoreRuntime{image:bhruntime.ImageIdentity{Reference:"postgres:18",Digest:"sha256:"+strings.Repeat("a",64)}}
 ops:=&coreNativeRuntimeOps{runtime:runtime,core:bhruntime.Files{Project:"owned-core",Compose:"core.yaml",Env:"core.env"},identity:identityprovider.KeycloakFiles{Project:"owned-identity"}}
 delta:=coreupdate.Delta{Installed:coreupdate.Realization{Kind:coreupdate.SQL,Instance:"postgres-member-1",Digest:"sha256:"+strings.Repeat("a",64)},Desired:coreupdate.Desired{Kind:coreupdate.SQL,Digest:"sha256:"+strings.Repeat("b",64)}}
 if err:=ops.ReconcilePinned(context.Background(),delta);err==nil{t.Fatal("wrong running PG digest accepted")}
 runtime.image.Digest=delta.Desired.Digest
 if err:=ops.ReconcilePinned(context.Background(),delta);err!=nil{t.Fatal(err)}
 if runtime.started!=2{t.Fatal("Core reconcile not executed")}
 if err:=ops.ReconcileOriginal(context.Background(),delta);err==nil{t.Fatal("failed to detect old digest mismatch")}
 if !errors.Is(context.Canceled,context.Canceled){t.Fatal("unreachable")}
}
