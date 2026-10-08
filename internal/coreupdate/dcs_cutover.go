package coreupdate

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
)

// DCSCutoverOps separates old-cluster fencing, isolated snapshot activation,
// and new-cluster/Patroni readiness. Each method must be an idempotent native
// RuntimeProvider action; no Compose project-wide stop is allowed.
type DCSCutoverOps interface {
 FenceOldDCS(context.Context) error
 VerifyFenced(context.Context) error
 ActivateIsolated(context.Context,DCSRecoveryEvidence) error
 VerifyNewQuorum(context.Context,DCSRecoveryEvidence) error
 VerifyPatroniDCS(context.Context) error
 CommitCutover(context.Context,DCSRecoveryEvidence) error
}

// DCSCutoverJournal stores the last durable phase in a protected Core state
// directory. A failed or ambiguous phase remains fenced and MUST NOT restore
// the previous DCS automatically, avoiding concurrent DCS primaries.
type DCSCutoverJournal struct {Path string}

func (j DCSCutoverJournal) load()(string,error){
 if j.Path==""{return "",errors.New("durable DCS cutover journal path required")}
 st,err:=os.Lstat(j.Path)
 if errors.Is(err,os.ErrNotExist){return "",nil}
 if err!=nil{return "",err}
 if !st.Mode().IsRegular()||st.Mode().Perm()&0077!=0||st.Size()>64{
  return "",errors.New("unsafe DCS cutover journal")
 }
 data,err:=os.ReadFile(j.Path);if err!=nil{return "",err}
 switch state:=strings.TrimSpace(string(data));state {
 case "prepared","fenced","activated","verified","committed":return state,nil
 default:return "",errors.New("corrupt or unknown DCS cutover state")
 }
}
func (j DCSCutoverJournal) record(previous,next string)error{
 if j.Path=="" {return errors.New("durable DCS cutover journal path required")}
 dir:=filepath.Dir(j.Path)
 info,err:=os.Lstat(dir);if err!=nil{return err}
 if !info.IsDir()||info.Mode().Perm()&0077!=0{return errors.New("DCS cutover journal directory not private")}
 observed,err:=j.load();if err!=nil{return err}
 if observed!=previous{return errors.New("DCS cutover phase changed unexpectedly")}
 tmp,err:=os.CreateTemp(dir,".dcs-cutover-");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err=tmp.WriteString(next+"\n");err==nil{err=tmp.Sync()}
 closeErr:=tmp.Close()
 if err!=nil{return err}
 if closeErr!=nil{return closeErr}
 if err=os.Rename(tmp.Name(),j.Path);err!=nil{return err}
 handle,err:=os.Open(dir);if err!=nil{return err};defer handle.Close()
 return handle.Sync()
}

// RunVerifiedDCSCutover MUST only run after PostgreSQL physical recovery and
// immutable DCS snapshot validation. A resumed 'prepared' state is ambiguous:
// an interrupted fence may already be active, so require operator recovery.
// A resumed 'fenced' state cannot activate without observed fencing.
func RunVerifiedDCSCutover(ctx context.Context,adapter DCSRecoveryAdapter,evidence DCSRecoveryEvidence,installation,target,cluster,release string,ops DCSCutoverOps,journal DCSCutoverJournal)error{
 if ops==nil{return ErrDCSUnsupported}
 if err:=VerifyDCSEvidence(ctx,adapter,evidence,installation,target,cluster,release);err!=nil{return err}
 phase,err:=journal.load();if err!=nil{return err}
 if phase==""{
  if err:=journal.record("","prepared");err!=nil{return err}
  // Prepare phase is journaled BEFORE an old-cluster fence.
  if err:=ops.FenceOldDCS(ctx);err!=nil{return fmt.Errorf("old DCS fence outcome ambiguous: %w",err)}
  if err:=ops.VerifyFenced(ctx);err!=nil{return fmt.Errorf("old DCS fence not proven: %w",err)}
  if err:=journal.record("prepared","fenced");err!=nil{return err}
  phase="fenced"
 }else if phase=="prepared"{
  return errors.New("UNSUPPORTED: interrupted DCS fencing requires operator verification; refusing automatic dual-primary cutover")
 }
 if err:=ops.VerifyFenced(ctx);err!=nil{return fmt.Errorf("DCS fence evidence lost: %w",err)}
 if phase=="fenced"{
  // After fencing the old cluster, restore and activate only the isolated,
  // previously validated snapshot. On failure the old cluster stays fenced.
  if err:=ops.ActivateIsolated(ctx,evidence);err!=nil{return fmt.Errorf("isolated DCS activation interrupted: %w",err)}
  if err:=journal.record("fenced","activated");err!=nil{return err}
  phase="activated"
 }
 if err:=ops.VerifyNewQuorum(ctx,evidence);err!=nil{return fmt.Errorf("new DCS quorum is unverified: %w",err)}
 if err:=ops.VerifyPatroniDCS(ctx);err!=nil{return fmt.Errorf("Patroni DCS readiness unverified: %w",err)}
 if phase=="activated"{
  if err:=journal.record("activated","verified");err!=nil{return err}
  phase="verified"
 }
 if phase=="verified"{
  if err:=ops.CommitCutover(ctx,evidence);err!=nil{return err}
  return journal.record("verified","committed")
 }
 if phase!="committed"{return fmt.Errorf("unsupported DCS phase %s",phase)}
 return nil
}
