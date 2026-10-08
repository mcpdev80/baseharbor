package coreupdate

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
)

// DCSCheckpoint binds a validated backup to the exact upgrade transaction.
// On resume it verifies the prior artifact instead of silently replacing it
// with a new snapshot from an already partially mutated cluster.
type DCSCheckpoint struct {Path string}

func (c DCSCheckpoint) Acquire(ctx context.Context, adapter DCSRecoveryAdapter, installation,cluster,release string)(DCSRecoveryEvidence,error){
 if c.Path=="" {return DCSRecoveryEvidence{},errors.New("durable DCS recovery receipt path required")}
 if adapter==nil{return DCSRecoveryEvidence{},ErrDCSUnsupported}
 dir:=filepath.Dir(c.Path)
 info,err:=os.Lstat(dir)
 if err!=nil{return DCSRecoveryEvidence{},err}
 if !info.IsDir()||info.Mode().Perm()&0077!=0{return DCSRecoveryEvidence{},errors.New("DCS receipt directory must be owner-only")}
 existing,err:=os.Lstat(c.Path)
 if err==nil {
  if !existing.Mode().IsRegular()||existing.Mode().Perm()&0077!=0{return DCSRecoveryEvidence{},errors.New("unsafe DCS receipt file")}
  f,e:=os.Open(c.Path);if e!=nil{return DCSRecoveryEvidence{},e}
  decoder:=json.NewDecoder(io.LimitReader(f,4096))
  decoder.DisallowUnknownFields()
  var evidence DCSRecoveryEvidence
  e=decoder.Decode(&evidence)
  if e==nil&&decoder.Decode(new(any))!=io.EOF{e=errors.New("trailing DCS receipt data")}
  f.Close()
  if e!=nil{return DCSRecoveryEvidence{},fmt.Errorf("corrupt DCS receipt: %w",e)}
  if e=VerifyDCSEvidence(ctx,adapter,evidence,installation,cluster,release);e!=nil{return DCSRecoveryEvidence{},e}
  return evidence,nil
 }
 if !errors.Is(err,os.ErrNotExist){return DCSRecoveryEvidence{},err}
 evidence,err:=CaptureAndVerifyDCS(ctx,adapter,installation,cluster,release)
 if err!=nil{return DCSRecoveryEvidence{},err}
 data,err:=json.Marshal(evidence);if err!=nil{return DCSRecoveryEvidence{},err}
 f,err:=os.OpenFile(c.Path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return DCSRecoveryEvidence{},err}
 if _,err=f.Write(append(data,'\n'));err==nil{err=f.Sync()}
 closeErr:=f.Close()
 if err!=nil{return DCSRecoveryEvidence{},err}
 if closeErr!=nil{return DCSRecoveryEvidence{},closeErr}
 handle,err:=os.Open(dir);if err!=nil{return DCSRecoveryEvidence{},err}
 defer handle.Close()
 if err=handle.Sync();err!=nil{return DCSRecoveryEvidence{},err}
 return evidence,nil
}
