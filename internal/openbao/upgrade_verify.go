package openbao

import (
 "context"
 "errors"
 "fmt"
 "strings"

 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// VerifyUpgradeManagerPolicyAndAppRole performs live, read-only authority
// verification before a Core OpenBao upgrade. Authentication is performed
// through protected credential files; tokens never enter command arguments.
func VerifyUpgradeManagerPolicyAndAppRole(ctx context.Context, executor Executor, files bhruntime.Files) error {
 if executor==nil || files.Project=="" || files.Compose=="" || files.Env=="" {
  return errors.New("managed OpenBao runtime is required")
 }
 state,err:=Inspect(ctx,executor,files)
 if err!=nil{return err}
 if !state.Initialized||state.Sealed{return errors.New("OpenBao manager verification requires initialized unsealed provider")}
 creds,err:=LoadAdminCredentials(files)
 if err!=nil{return err}
 token,err:=loginManager(ctx,executor,files,creds)
 if err!=nil{return errors.New("OpenBao manager AppRole cannot authenticate")}
 checks:=[]struct{name,command string}{
  {"manager AppRole","exec bao read -format=json auth/approle/role/baseharbor-manager"},
  {"manager policy","exec bao policy read baseharbor-manager"},
  {"manager KV access","exec bao kv list -format=json -mount=baseharbor /"},
 }
 for _,check:=range checks {
  response,err:=execWithToken(ctx,executor,files,token,check.command)
  if err!=nil{return fmt.Errorf("OpenBao %s authorization failed",check.name)}
  if strings.TrimSpace(response)=="" {return fmt.Errorf("OpenBao %s returned no observable result",check.name)}
 }
 return nil
}

// VerifyUpgradeApplicationScopes validates every explicitly owned application
// AppRole and its secret access without creating or changing any credentials.
func VerifyUpgradeApplicationScopes(ctx context.Context,executor Executor,files bhruntime.Files,scopes map[ApplicationIdentity]string)error{
 if executor==nil||len(scopes)==0{return errors.New("application scope inventory required")}
 for identity,credentialsPath:=range scopes {
  if credentialsPath=="" {return errors.New("application scope credentials unavailable")}
  if err:=CheckApplicationScope(ctx,executor,files,identity,credentialsPath);err!=nil{
   return fmt.Errorf("OpenBao application %s/%s secret scope not verified: %w",identity.Name,identity.Environment,err)
  }
 }
 return nil
}
