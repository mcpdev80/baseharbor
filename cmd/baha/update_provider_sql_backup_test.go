package main

import (
 "archive/tar"
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/coreupdate"
)

func TestProviderConfigurationRestoreReplaysAndRejectsTampering(t *testing.T) {
 dir:=t.TempDir()
 if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
 config:=filepath.Join(dir,"config.env")
 original:=[]byte("SECRET=original\n")
 if err:=os.WriteFile(config,original,0600);err!=nil{t.Fatal(err)}
 spec:=providerSQLBackupSpec{Directory:filepath.Join(dir,"backup"),Name:"openbao",ConfigPaths:[]string{config}}
 point:=coreupdate.StreamRecoveryPoint{Directory:spec.Directory,Name:"openbao-config"}
 err:=point.Capture(context.Background(),func(ctx context.Context,w io.Writer)error{
  tw:=tar.NewWriter(w)
  if err:=tw.WriteHeader(&tar.Header{Name:"0",Mode:0600,Size:int64(len(original)),Typeflag:tar.TypeReg});err!=nil{return err}
  if _,err:=tw.Write(original);err!=nil{return err}
  return tw.Close()
 })
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(config,[]byte("SECRET=changed\n"),0600);err!=nil{t.Fatal(err)}
 if err:=spec.restoreConfiguration(context.Background());err!=nil{t.Fatal(err)}
 got,err:=os.ReadFile(config);if err!=nil{t.Fatal(err)}
 if string(got)!=string(original){t.Fatalf("configuration not recovered: %q",got)}
 if err:=spec.restoreConfiguration(context.Background());err!=nil{t.Fatalf("idempotent replay failed: %v",err)}
 if err:=os.WriteFile(config,[]byte("SECRET=changed-again\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Chmod(config,0666);err!=nil{t.Fatal(err)}
 if err:=spec.restoreConfiguration(context.Background());err==nil{t.Fatal("unsafe destination accepted")}
 got,err=os.ReadFile(config);if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(got),"changed-again"){t.Fatal("unsafe destination unexpectedly mutated")}
 if err:=os.Chmod(config,0600);err!=nil{t.Fatal(err)}
 checksum:=filepath.Join(spec.Directory,"openbao-config.backup.sha256")
 if err:=os.WriteFile(checksum,[]byte(strings.Repeat("0",64)),0600);err!=nil{t.Fatal(err)}
 if err:=spec.restoreConfiguration(context.Background());err==nil{t.Fatal("tampered archive accepted")}
 if _,err:=os.Stat(filepath.Join(dir,"missing"));!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
}
