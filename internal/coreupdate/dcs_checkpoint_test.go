package coreupdate

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

type fakeDurableDCS struct {snapshots int;evidence DCSRecoveryEvidence}
func (f *fakeDurableDCS) Snapshot(context.Context)(DCSRecoveryEvidence,error){f.snapshots++;return f.evidence,nil}
func (f *fakeDurableDCS) Validate(context.Context,DCSRecoveryEvidence)error{return nil}
func (f *fakeDurableDCS) VerifyRestorable(context.Context,DCSRecoveryEvidence)error{return nil}
func (f *fakeDurableDCS) Restore(context.Context,DCSRecoveryEvidence)error{return ErrDCSUnsupported}

func TestDCSCheckpointPersistsAndRejectsTamper(t *testing.T){
 dir:=t.TempDir()
 if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
 receipt:=DCSCheckpoint{Path:filepath.Join(dir,"dcs.json")}
 fake:=&fakeDurableDCS{evidence:DCSRecoveryEvidence{Installation:"core",Cluster:"db",Release:"0.4.24",SnapshotID:"snapshot-1",SHA256:strings.Repeat("a",64)}}
 for i:=0;i<2;i++{
  got,err:=receipt.Acquire(context.Background(),fake,"core","db","0.4.24")
  if err!=nil||got.SnapshotID!="snapshot-1"{t.Fatalf("checkpoint attempt %d: %+v err=%v",i,got,err)}
 }
 if fake.snapshots!=1{t.Fatalf("recreated snapshot on resume: %d",fake.snapshots)}
 if _,err:=receipt.Acquire(context.Background(),fake,"foreign","db","0.4.24");err==nil{t.Fatal("foreign owner reused backup")}
 if err:=os.WriteFile(receipt.Path,[]byte("{\"invalid\":true}"),0600);err!=nil{t.Fatal(err)}
 if _,err:=receipt.Acquire(context.Background(),fake,"core","db","0.4.24");err==nil{t.Fatal("corrupt DCS receipt accepted")}
 if fake.snapshots!=1{t.Fatal("corrupt evidence silently recreated")}
}
