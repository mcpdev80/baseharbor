package main

import (
 "encoding/json"
 "bytes"
 "context"
 "github.com/mcpdev80/baseharbor/internal/operatorauth"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestGuidedBackupDefaultOutsideGitWorktree(t *testing.T) {
 home:=t.TempDir()
 t.Setenv("HOME",home)
 t.Setenv("XDG_DATA_HOME",filepath.Join(home,"data"))
 git:=filepath.Join(home,"repo")
 if err:=os.MkdirAll(filepath.Join(git,".git"),0700);err!=nil{t.Fatal(err)}
 path,err:=defaultGuidedBackupPath("demo","dev",time.Date(2026,10,8,12,0,0,0,time.UTC))
 if err!=nil{t.Fatal(err)}
 rel,err:=filepath.Rel(git,path)
 if err!=nil{t.Fatal(err)}
 if rel!=".."&&!strings.HasPrefix(rel,".."+string(os.PathSeparator)){t.Fatalf("archive inside repo: %s",path)}
 if filepath.Ext(path)!=".bhbackup"{t.Fatal(path)}
 info,err:=os.Stat(filepath.Dir(path))
 if err!=nil||info.Mode().Perm()!=0700{t.Fatalf("directory mode %v err %v",info,err)}
}

func TestAppShowMachineShape(t *testing.T){
 b,err:=json.Marshal(applicationOverview{ContractVersion:"v1",Postgres:[]overviewResource{},Valkey:[]overviewResource{}})
 if err!=nil{t.Fatal(err)}
 got:=string(b)
 for _,expected:=range []string{`"contract_version":"v1"`,`"name":""`,`"postgres":[]`,`"valkey":[]`}{
  if !strings.Contains(got,expected){t.Fatalf("missing %s in %s",expected,got)}
 }
 if strings.Contains(got,`"Name"`)||strings.Contains(got,`"postgres":null`)||strings.Contains(got,`"valkey":null`){t.Fatal(got)}
}

func TestAppInitUsageIncludesDeterministicWorkloadFlags(t *testing.T){
 command:=rootCommand()
 var appUsage string
 for _,child:=range command.Children{
  if child.Name=="app" {
   for _,nested:=range child.Children{if nested.Name=="init"{appUsage=nested.Usage}}
  }
 }
 for _,flag:=range []string{"--workload-component","--workload-source"}{if !strings.Contains(appUsage,flag){t.Errorf("help missing %s: %s",flag,appUsage)}}
}

func TestOperatorAuthConfigurationDoesNotPromptNonTTY(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 input,writer,err:=os.Pipe()
 if err!=nil{t.Fatal(err)}
 defer input.Close()
 defer writer.Close()
 var out bytes.Buffer
 ctx:=operatorauth.WithInteractive(operatorauth.WithEnforcement(context.Background()),input,&out,&out)
 _,err=resolveOperatorAuthBoundaryConfig(ctx,"local","prod")
 if err==nil{t.Fatal("expected explicit auth configuration error in non TTY mode")}
 if strings.Contains(out.String(),"Configure it now?"){t.Fatalf("unexpected prompt on non TTY: %s",out.String())}
}
