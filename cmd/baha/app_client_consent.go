package main

import (
 "bufio"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "time"

 "github.com/mcpdev80/baseharbor/internal/deployment"
)

const managedClientConsentLifetime = 12 * time.Hour

// Permission to start one client is distinct from service authorization.
// Authentication and resource-specific authorization are checked afresh on each call.
func requireManagedClientConsent(ctx context.Context, in io.Reader, out io.Writer, target, application, environment, kind, instance, execution string) error {
 if target=="" || application=="" || environment=="" || kind=="" || instance=="" || execution=="" {
  return errors.New("cannot request consent for an incomplete client scope")
 }
 scope:= strings.Join([]string{target,application,environment,kind,instance,execution},"\x00")
 digest:=sha256.Sum256([]byte(scope))
 id:=hex.EncodeToString(digest[:])
 configPath,err:=deployment.ConfigPath()
 if err!=nil{return err}
 path:=filepath.Join(filepath.Dir(configPath),"client-consent",id)
 if info,err:=os.Lstat(path);err==nil{
  if !info.Mode().IsRegular()|| info.Mode().Perm()&0077!=0{return errors.New("client consent record is insecure; inspect and remove it before retrying")}
  data,err:=os.ReadFile(path)
  if err!=nil{return err}
  until,err:=time.Parse(time.RFC3339,strings.TrimSpace(string(data)))
  if err==nil && time.Now().Before(until){return nil}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 fmt.Fprintf(out,"Client: %s\nApp: %s / %s\nTarget: %s\nInstance: %s\nExecution: %s\nSession: up to 12 hours, only for this scope; server authorization checked every time.\n",kind,application,environment,target,instance,execution)
 if noInput(ctx)||in==nil||!readerIsTerminal(in){
  return usageError("explicit consent is required before launching a client", "Run this command in a terminal to approve the exact client, instance, Target and execution method.")
 }
 fmt.Fprint(out,"Approve this client session for 12 hours? [y/N]: ")
 answer,err:=bufio.NewReader(in).ReadString('\n')
 if err!=nil{return fmt.Errorf("read client consent: %w",err)}
 if strings.ToLower(strings.TrimSpace(answer))!="y" && strings.ToLower(strings.TrimSpace(answer))!="yes"{
  return usageError("client launch was not approved", "No client was started. Re-run to approve or choose another method.")
 }
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".consent-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err:=f.WriteString(time.Now().Add(managedClientConsentLifetime).UTC().Format(time.RFC3339)+"\n");err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}
