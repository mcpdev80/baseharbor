package coreupdate

import (
 "context"
 "errors"
 "fmt"
 "os"

 etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
)

// EtcdDCSBridge adapts the isolated Session 1B snapshot/restore store to the
// central Core journal. The restore destination must be a new, isolated path:
// this bridge never replaces a live etcd member's data directory.
type EtcdDCSBridge struct {
 Store etcdbackup.Store
 Restorer etcdbackup.Restorer
 RecoveryDirectory string
 Release string
 // VerifyRecoveredCluster must attest isolated etcd startup, authenticated
 // cluster identity, quorum and Patroni DCS readiness. File presence is insufficient.
 VerifyRecoveredCluster func(context.Context,etcdbackup.Identity,etcdbackup.SnapshotInfo) error
}

func (b *EtcdDCSBridge) evidence(m etcdbackup.Metadata) DCSRecoveryEvidence {
 return DCSRecoveryEvidence{
  Installation:m.Identity.Core, Target:m.Identity.Target, Cluster:m.Identity.Cluster,
  Release:b.Release, SnapshotID:"etcd.snapshot", SHA256:m.SHA256,
 }
}
func (b *EtcdDCSBridge) Snapshot(ctx context.Context)(DCSRecoveryEvidence,error) {
 if b==nil||b.Release==""||b.Restorer==nil||b.RecoveryDirectory=="" {return DCSRecoveryEvidence{},ErrDCSUnsupported}
 meta,err:=b.Store.CreateSnapshot(ctx);if err!=nil{return DCSRecoveryEvidence{},err}
 return b.evidence(meta),nil
}
func (b *EtcdDCSBridge) Validate(ctx context.Context, ev DCSRecoveryEvidence)error {
 if b==nil||b.Release=="" {return ErrDCSUnsupported}
 meta,err:=b.Store.VerifySnapshot(ctx);if err!=nil{return err}
 if b.evidence(meta)!=ev {return ErrDCSInvalidEvidence}
 return nil
}
func (b *EtcdDCSBridge) VerifyRestorable(ctx context.Context, ev DCSRecoveryEvidence)error {
 if err:=b.Validate(ctx,ev);err!=nil{return err}
 if b.Restorer==nil||b.RecoveryDirectory==""||b.VerifyRecoveredCluster==nil {return ErrDCSUnsupported}
 plan,err:=b.Store.PrepareRestore(ctx,b.RecoveryDirectory)
 if err==nil {
  if err:=b.Store.Restore(ctx,plan,b.Restorer);err!=nil{return fmt.Errorf("isolated etcd recovery proof: %w",err)}
  if err:=b.Store.VerifyRecovery(ctx,plan,b.Restorer);err!=nil{return err}
  return b.VerifyRecoveredCluster(ctx,plan.Identity,plan.Info)
 }
 // A prior verified isolated directory is reused on resume, not overwritten.
 if _,statErr:=os.Lstat(b.RecoveryDirectory);statErr!=nil{return fmt.Errorf("isolated etcd restore unavailable: %w",err)}
 meta,metaErr:=b.Store.VerifySnapshot(ctx);if metaErr!=nil{return metaErr}
 // VerifyRecovery re-checks checksum and restore identity before delegation.
 archive:=b.Store.Directory+"/etcd.snapshot"
 plan=etcdbackup.RestorePlan{Archive:archive,Destination:b.RecoveryDirectory,Identity:b.Store.Identity,Info:meta.Info,SHA256:meta.SHA256}
 if verifyErr:=b.Store.VerifyRecovery(ctx,plan,b.Restorer);verifyErr!=nil{return fmt.Errorf("prior isolated etcd restore is unverified: %w",verifyErr)}
 meta,metaErr=b.Store.VerifySnapshot(ctx);if metaErr!=nil{return metaErr}
 return b.VerifyRecoveredCluster(ctx,b.Store.Identity,meta.Info)
}
func (b *EtcdDCSBridge) Restore(ctx context.Context,ev DCSRecoveryEvidence)error {
 if b==nil{return ErrDCSUnsupported}
 if err:=b.Validate(ctx,ev);err!=nil{return err}
 // Destructive live-cluster replacement is never implied by this API.
 return errors.New("UNSUPPORTED: online DCS replacement requires fenced member lifecycle and isolated restore cutover proof")
}
