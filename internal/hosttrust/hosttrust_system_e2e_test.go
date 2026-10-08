package hosttrust

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"
)

// This test MUST only run on an ephemeral, disposable Linux CI runner.
// It mutates the real machine trust store and requires an explicit opt-in.
func TestPrivilegedSystemHostTrustInstallUninstallE2E(t *testing.T) {
 if os.Getenv("BASEHARBOR_HOST_TRUST_E2E")!="1" {t.Skip("privileged host-trust E2E requires explicit opt-in")}
 if runtime.GOOS!="linux" {t.Fatal("requires disposable Linux host")}
 if _,err:=exec.LookPath("update-ca-certificates");err!=nil {t.Skip("Ubuntu/Debian CA backend unavailable")}
 dir:=t.TempDir()
 ca:=testCA(t,"BaseHarbor temporary host trust E2E "+filepath.Base(dir))
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
 defer cancel()
 backend,err:=DetectSystemBackend()
 if err!=nil{t.Fatal(err)}
 status,err:=Install(ctx,dir,ca,"e2e://ephemeral-ci",backend)
 if err!=nil{t.Fatal(err)}
 // Cleanup even if verification fails; this runner must be disposable.
 defer func() {
  if _,err:=RemoveOwned(context.Background(),dir);err!=nil {t.Errorf("cleanup temporary CI CA: %v",err)}
 }()
 if !strings.HasPrefix(status.Path,"/usr/local/share/ca-certificates/baseharbor-") {t.Fatalf("unexpected system trust path %s",status.Path)}
 if _,err:=os.Stat(status.Path);err!=nil{t.Fatal(err)}
 result,err:=RemoveOwnedDetailed(ctx,dir)
 if err!=nil{t.Fatal(err)}
 if len(result.Removed)!=1 || len(result.Preserved)!=0 {t.Fatalf("uninstall report %+v",result)}
 if _,err:=os.Lstat(status.Path);!os.IsNotExist(err) {t.Fatalf("system anchor survived uninstall: %v",err)}
 records,err:=StateRecords(dir)
 if err!=nil{t.Fatal(err)}
 if len(records)!=0{t.Fatalf("ownership records survived uninstall: %+v",records)}
 result,err=RemoveOwnedDetailed(ctx,dir)
 if err!=nil || len(result.Removed)!=0 {t.Fatalf("uninstall not idempotent: %+v %v",result,err)}
}
