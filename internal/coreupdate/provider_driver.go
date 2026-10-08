package coreupdate

import(
 "context"
 "errors"
 "fmt"
)

// NativeProviderOps holds provider-specific lifecycle operations (PostgreSQL,
// OpenBao and Keycloak). It never guesses database quiescence or OIDC readiness.
type NativeProviderOps interface{
 Preflight(context.Context,Plan)error
 Quiesce(context.Context,Delta)error
 ReconcilePinned(context.Context,Delta)error
 ReconcileOriginal(context.Context,Delta)error
 VerifySemantics(context.Context,Delta)error
 Record(context.Context,Delta,string)error
}
type NativeProviderAssets struct{
 Recovery VolumeRecovery
 Compose ComposeCheckpoint
}

// RunNativeProviderUpdates wires immutable provider-volume recovery, atomic
// pinned Compose staging, provider-native lifecycle and semantic verification
// into the durable journal. A missing hook or per-provider asset fails before
// the first provider is touched.
func RunNativeProviderUpdates(ctx context.Context,plan Plan,journalPath string,ops NativeProviderOps,assets map[string]NativeProviderAssets)error{
 if ops==nil{return errors.New("Core provider native operations are required")}
 if journalPath==""{return errors.New("durable Core update journal path required")}
 for _,delta:=range plan.Deltas{
  if delta.Classification==NoChange{continue}
  entry,ok:=assets[JournalKey(delta)]
  if !ok||entry.Recovery.Runtime==nil||entry.Recovery.VerifyQuiesced==nil||
   entry.Compose.Path==""||entry.Compose.Directory==""{
   return fmt.Errorf("Core %s missing provider-native recovery and Compose assets",delta.Installed.Instance)
  }
 }
 hooks:=Hooks{
  Preflight:ops.Preflight,
  RecoveryPoint:func(ctx context.Context,d Delta)error{
   a:=assets[JournalKey(d)]
   if err:=ops.Quiesce(ctx,d);err!=nil{return err}
   return a.Recovery.Capture(ctx,d)
  },
  Recover:func(ctx context.Context,d Delta,_ string)error{
   a:=assets[JournalKey(d)]
   if err:=ops.Quiesce(ctx,d);err!=nil{return err}
   if err:=a.Recovery.Recover(ctx,d);err!=nil{return err}
   if err:=a.Compose.Restore(map[string]Delta{d.Installed.Instance:d});err!=nil{return err}
   if err:=ops.ReconcileOriginal(ctx,d);err!=nil{return err}
   return ops.VerifySemantics(ctx,d)
  },
  Apply:func(ctx context.Context,d Delta)error{
   a:=assets[JournalKey(d)]
   if err:=a.Compose.Stage(map[string]Delta{d.Installed.Instance:d});err!=nil{return err}
   return ops.ReconcilePinned(ctx,d)
  },
  Verify:ops.VerifySemantics,
  Record:ops.Record,
 }
 return ExecuteJournaled(ctx,plan,journalPath,hooks)
}
