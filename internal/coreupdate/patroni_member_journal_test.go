package coreupdate

import (
 "context"
 "os"
 "path/filepath"
 "testing"
)

func TestPatroniMemberJournalDurableResume(t *testing.T){
 dir:=t.TempDir()
 if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
 journal:=PatroniMemberJournal{Path:filepath.Join(dir,"journal.json"),Release:"0.4.24",Installation:"core",Scope:"shared",Desired:Desired{Kind:SQL,Image:"ghcr.io/zalando/spilo-18:4.1-p2",Version:"18-spilo-4.1-p2",Digest:digestA}}
 ctx:=context.Background()
 if state,err:=journal.StepState(ctx,"postgres-member-2");err!=nil||state!=""{t.Fatalf("unexpected state %q: %v",state,err)}
 if err:=journal.Record(ctx,"postgres-member-2","applying");err!=nil{t.Fatal(err)}
 if state,err:=journal.StepState(ctx,"postgres-member-2");err!=nil||state!="applying"{t.Fatalf("resume did not persist: %q %v",state,err)}
 if err:=journal.Record(ctx,"postgres-member-2","verified");err!=nil{t.Fatal(err)}
 if err:=journal.Record(ctx,"postgres-member-2","applying");err==nil{t.Fatal("verified member regressed")}
 other:=journal
 other.Desired.Digest=digestB
 if state,err:=other.StepState(ctx,"postgres-member-2");err!=nil||state!=""{t.Fatalf("stale release image reused: %q %v",state,err)}
 if err:=journal.Record(ctx,"foreign-member","applying");err==nil{t.Fatal("foreign member accepted")}
}
