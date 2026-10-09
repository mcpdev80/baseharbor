package coreupdate

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestMixedUpdatePreflightFailurePreservesExistingInterruptedProvider(t *testing.T) {
 dir := t.TempDir()
 if err := os.Chmod(dir, 0700); err != nil { t.Fatal(err) }
 delta := Delta{Installed: Realization{Kind: Secrets, Installation: "c", Scope: "shared", Instance: "openbao-member-1", Owner: "baseharbor", Image: "bao:2.7.0", Version: "2.7.0", Digest: digestA}, Desired: Desired{Kind: Secrets, Image: "bao:2.7.1", Version: "2.7.1", Digest: digestB}, Classification: BackupRequired}
 journalPath := filepath.Join(dir,"journal.json")
 journal := Journal{Release:"0.4.24"}
 if err := journal.Record(journalPath,delta,"apply_failed"); err != nil { t.Fatal(err) }
 recovered := false
 bound := Hooks{
  Preflight: func(context.Context,Plan)error{return errors.New("ownership preflight rejected")},
  RecoveryPoint:func(context.Context,Delta)error{return nil},
  Apply:func(context.Context,Delta)error{return nil},
  Verify:func(context.Context,Delta)error{return nil},
  Recover:func(context.Context,Delta,string)error{recovered=true;return nil},
 }
 ops := &mockNativeOps{}
 err := RunMixedProviderUpdates(context.Background(),Plan{Release:"0.4.24",Deltas:[]Delta{delta}},journalPath,ops,nil,bound,func(Delta)bool{return true})
 if err == nil { t.Fatal("preflight failure accepted") }
 if recovered {t.Fatal("failed preflight triggered unsafe rollback of previously interrupted provider")}
 after,err := LoadJournal(journalPath,"0.4.24")
 if err != nil {t.Fatal(err)}
 if after.Steps[JournalKey(delta)]!="apply_failed" {t.Fatalf("interrupted state lost: %+v",after.Steps)}
}
