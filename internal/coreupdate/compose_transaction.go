package coreupdate

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

// ComposeCheckpoint makes the on-disk image update reversible after an
// already verified provider backup was captured. It cannot replace recovery
// of data volumes; callers must pair it with a native provider recovery point.
type ComposeCheckpoint struct{
 Path string
 Directory string
}

func(c ComposeCheckpoint) checkpointName(mutations map[string]Delta)(string,error){
 if c.Path==""||c.Directory==""||len(mutations)==0{return "",errors.New("Compose checkpoint requires a file, directory and provider deltas")}
 names:=make([]string,0,len(mutations));for n:=range mutations{names=append(names,n)}
 sort.Strings(names)
 var b strings.Builder
 for _,n:=range names{b.WriteString(n);b.WriteByte(0);b.WriteString(JournalKey(mutations[n]));b.WriteByte(0)}
 hash:=sha256.Sum256([]byte(b.String()))
 return filepath.Join(c.Directory,hex.EncodeToString(hash[:])+".compose"),nil
}

func readVerifiedCompose(path string)([]byte,error){
 st,err:=os.Lstat(path)
 if err!=nil{return nil,err}
 if !st.Mode().IsRegular()||st.Mode().Perm()&0077!=0{return nil,fmt.Errorf("Compose %s not an owner-only regular file",path)}
 return os.ReadFile(path)
}
func writeExclusive(path string,data []byte)error{
 f,err:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if err!=nil{return err}
 defer f.Close()
 if _,err:=f.Write(data);err!=nil{return err}
 return f.Sync()
}
func atomicReplaceOwnerFile(path string,data []byte)error{
 dir:=filepath.Dir(path)
 f,err:=os.CreateTemp(dir,".core-image-*")
 if err!=nil{return err}
 tmp:=f.Name();defer os.Remove(tmp)
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err:=f.Write(data);err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 if _,err:=readVerifiedCompose(path);err!=nil{return err}
 if err:=os.Rename(tmp,path);err!=nil{return err}
 d,err:=os.Open(dir);if err!=nil{return err};defer d.Close()
 return d.Sync()
}

// Stage preserves the original Compose atomically before writing the new,
// immutable image-pinned realization. A replay verifies the prior checkpoint
// rather than creating a second backup from possibly mutated Compose.
func(c ComposeCheckpoint) Stage(mutations map[string]Delta)error{
 checkpoint,err:=c.checkpointName(mutations);if err!=nil{return err}
 current,err:=readVerifiedCompose(c.Path);if err!=nil{return err}
 if err:=os.MkdirAll(c.Directory,0700);err!=nil{return err}
 if err:=os.Chmod(c.Directory,0700);err!=nil{return err}
 original,err:=readVerifiedCompose(checkpoint)
 if errors.Is(err,os.ErrNotExist){
  original=append([]byte(nil),current...)
  pinned,rewriteErr:=RewriteOwnedComposeImages(original,mutations);if rewriteErr!=nil{return rewriteErr}
  if err:=writeExclusive(checkpoint,original);err!=nil{return err}
  return atomicReplaceOwnerFile(c.Path,pinned)
 }
 if err!=nil{return fmt.Errorf("unsafe existing Compose checkpoint: %w",err)}
 pinned,err:=RewriteOwnedComposeImages(original,mutations);if err!=nil{return err}
 if bytes.Equal(current,pinned){return nil}
 if !bytes.Equal(current,original){return errors.New("Compose differs from both pre-upgrade and pinned state; refusing overwrite")}
 return atomicReplaceOwnerFile(c.Path,pinned)
}

// Restore returns to the exact original provider declarations when the runtime
// has been quiesced and data has been recovered by a provider-native hook.
func(c ComposeCheckpoint) Restore(mutations map[string]Delta)error{
 checkpoint,err:=c.checkpointName(mutations);if err!=nil{return err}
 original,err:=readVerifiedCompose(checkpoint);if err!=nil{return err}
 current,err:=readVerifiedCompose(c.Path);if err!=nil{return err}
 pinned,err:=RewriteOwnedComposeImages(original,mutations);if err!=nil{return err}
 if !bytes.Equal(current,original)&&!bytes.Equal(current,pinned){return errors.New("cannot restore Compose modified by another owner")}
 if bytes.Equal(current,original){return nil}
 return atomicReplaceOwnerFile(c.Path,original)
}
