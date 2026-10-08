package coreupdate

import (
 "archive/tar"
 "context"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestVerifyPostgresBasebackupMarkers(t *testing.T) {
 cases:=[]struct{name,major string;entries map[string]string;valid bool}{
 {"valid","18",map[string]string{"PG_VERSION":"18\n","backup_label":"START WAL LOCATION: 0/1000000\n","global/pg_control":"control"},true},
 {"wrong_major","18",map[string]string{"PG_VERSION":"17\n","backup_label":"label"},false},
 {"missing_label","18",map[string]string{"PG_VERSION":"18\n"},false},
 {"tablespaces","18",map[string]string{"PG_VERSION":"18\n","backup_label":"label","tablespace_map":"16384 /data"},false},
 {"not_tar","18",nil,false},
 }
 for _,tt:=range cases { t.Run(tt.name,func(t *testing.T) {
  dir:=t.TempDir()
  if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
  p:=StreamRecoveryPoint{Directory:filepath.Join(dir,"recovery"),Name:"pg"}
  err:=p.Capture(context.Background(),func(_ context.Context,w io.Writer)error {
   if tt.entries==nil { _,err:=io.Copy(w,strings.NewReader("not a real archive"));return err }
   tw:=tar.NewWriter(w)
   for name,value:=range tt.entries {
    if err:=tw.WriteHeader(&tar.Header{Name:name,Mode:0600,Size:int64(len(value)),Typeflag:tar.TypeReg});err!=nil{return err}
    if _,err:=io.WriteString(tw,value);err!=nil{return err}
   }
   return tw.Close()
  })
  if err!=nil{t.Fatal(err)}
  err=VerifyPostgresBasebackup(p,tt.major)
  if (err==nil)!=tt.valid{t.Fatalf("valid=%v err=%v",tt.valid,err)}
 }) }
}
