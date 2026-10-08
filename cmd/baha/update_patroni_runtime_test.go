package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/coreupdate"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type patroniRollingTestRuntime struct {
 bhruntime.RuntimeProvider
 recreated []string
 execService string
 digest string
}
func (f *patroniRollingTestRuntime) UpProjectFilesSelectedForceRecreateNoBuild(_ context.Context,_ string,_ string,_ map[string]string,services []string,_ ...string)error{
 f.recreated=append(f.recreated,services...)
 return nil
}
func (f *patroniRollingTestRuntime) ProjectServiceImageIdentity(_ context.Context,_,_ string)(bhruntime.ImageIdentity,error){
 return bhruntime.ImageIdentity{Reference:"ghcr.io/zalando/spilo-18:4.1-p2",Digest:f.digest},nil
}
func (f *patroniRollingTestRuntime) ExecProject(_ context.Context,_,_,_,service string,args ...string)(string,error){
 f.execService=service
 if len(args)<4||args[0]!="python3"||!strings.Contains(args[2],"switchover"){return "",errors.New("unexpected command")}
 return "",nil
}
func TestPatroniRollingRuntimeNeverMutatesForeignMember(t *testing.T){
 dir:=t.TempDir()
 env:=filepath.Join(dir,"runtime.env")
 if err:=os.WriteFile(env,[]byte("CONTROL=test\n"),0600);err!=nil{t.Fatal(err)}
 rt:=&patroniRollingTestRuntime{digest:"sha256:"+strings.Repeat("a",64)}
 files:=bhruntime.Files{HA:true,Project:"owned",Compose:filepath.Join(dir,"compose.yaml"),Env:env}
 ops:=&patroniCoreRollingOps{runtime:rt,files:files,journal:coreupdate.PatroniMemberJournal{Desired:coreupdate.Desired{Kind:coreupdate.SQL,Digest:rt.digest}}}
 if err:=ops.Recreate(context.Background(),"other-project-member");err==nil{t.Fatal("foreign replica mutated")}
 if len(rt.recreated)!=0{t.Fatal("unexpected mutation")}
 if err:=ops.Recreate(context.Background(),"postgres-member-2");err!=nil{t.Fatal(err)}
 if len(rt.recreated)!=1||rt.recreated[0]!="postgres-member-2"{t.Fatalf("not a targeted member restart: %v",rt.recreated)}
 if err:=ops.VerifyMemberImage(context.Background(),"postgres-member-2");err!=nil{t.Fatal(err)}
 rt.digest="sha256:"+strings.Repeat("b",64)
 if err:=ops.VerifyMemberImage(context.Background(),"postgres-member-2");err==nil{t.Fatal("image drift accepted")}
 if err:=ops.Switchover(context.Background(),"postgres-member-1","postgres-member-3");err!=nil{t.Fatal(err)}
 if rt.execService!="postgres-member-1"{t.Fatal("Patroni API not called on current leader")}
 if err:=ops.Switchover(context.Background(),"postgres-member-1","unknown");err==nil{t.Fatal("foreign candidate accepted")}
}
