package main
import(
 "context"
 "os"
 "path/filepath"
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
}

func TestNativeCoreReceiptsPreserveEveryProviderStep(t *testing.T){
 dir:=t.TempDir()
 if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
 ops:=&coreNativeRuntimeOps{release:"0.4.24",receiptPath:filepath.Join(dir,"receipts.json")}
 first:=coreupdate.Delta{Installed:coreupdate.Realization{Kind:coreupdate.SQL,Installation:"core",Scope:"shared",Instance:"postgres-member-1",Owner:"baseharbor"},Desired:coreupdate.Desired{Kind:coreupdate.SQL,Image:"postgres:18",Version:"18",Digest:"sha256:"+strings.Repeat("a",64)}}
 second:=coreupdate.Delta{Installed:coreupdate.Realization{Kind:coreupdate.Secrets,Installation:"core",Scope:"shared",Instance:"openbao-member-1",Owner:"baseharbor"},Desired:coreupdate.Desired{Kind:coreupdate.Secrets,Image:"bao:2.7",Version:"2.7",Digest:"sha256:"+strings.Repeat("b",64)}}
 if err:=ops.Record(context.Background(),first,"verified");err!=nil{t.Fatal(err)}
 if err:=ops.Record(context.Background(),second,"applying");err!=nil{t.Fatal(err)}
 journal,err:=coreupdate.LoadJournal(ops.receiptPath,ops.release);if err!=nil{t.Fatal(err)}
 if len(journal.Steps)!=2||journal.Steps[coreupdate.JournalKey(first)]!="verified"||journal.Steps[coreupdate.JournalKey(second)]!="applying"{
  t.Fatalf("Core provider receipt lost prior migration evidence: %+v",journal.Steps)
 }
}
