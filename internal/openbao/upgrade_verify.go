package openbao

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"

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
 roleJSON,err:=execWithToken(ctx,executor,files,token,"exec bao read -format=json auth/approle/role/baseharbor-manager/role-id")
 if err!=nil{return errors.New("OpenBao manager AppRole identity is unreadable")}
 var role struct {Data struct {RoleID string `json:"role_id"`} `json:"data"`}
 if err:=json.Unmarshal([]byte(roleJSON),&role);err!=nil||role.Data.RoleID==""||role.Data.RoleID!=creds.RoleID{return errors.New("OpenBao manager RoleID mismatches protected credentials")}
 capsJSON,err:=execWithToken(ctx,executor,files,token,"exec bao token capabilities -format=json sys/policies/acl/baseharbor-app-upgrade-probe")
 if err!=nil{return errors.New("OpenBao manager application-policy authorization failed")}
 if err:=verifyUpgradePolicyCapabilities(capsJSON);err!=nil{return err}
 if err:=verifyManagerKV(ctx,executor,files,token);err!=nil{return errors.New("OpenBao manager cannot verify protected KV access")}
 return nil
}


func verifyUpgradePolicyCapabilities(payload string)error{
 var caps []string
 if err:=json.Unmarshal([]byte(payload),&caps);err!=nil{return errors.New("OpenBao policy capability report is malformed")}
 available:=map[string]bool{}
 for _,capability:=range caps{available[capability]=true}
 for _,required:=range []string{"create","update","read","delete"} {
  if !available[required]{return fmt.Errorf("OpenBao manager lacks required %s application-policy authorization",required)}
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
