package etcd

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
type fakeSource struct { data string; err error; cluster string }
func (f fakeSource) Snapshot(_ context.Context,w io.Writer)(SnapshotInfo,error){
 if f.err!=nil{return SnapshotInfo{},f.err}
 _,err:=io.WriteString(w,f.data);return SnapshotInfo{ClusterID:f.cluster,Version:"3.6.0",Revision:42},err
}
type fakeRestore struct {called int}
func(f *fakeRestore) RestoreIsolated(_ context.Context,archive,dest string,_ Identity,_ SnapshotInfo)error{
 f.called++;return os.Mkdir(dest,0700)
}
func(f *fakeRestore) VerifyIsolated(_ context.Context,dest string,_ Identity,_ SnapshotInfo)error{
 st,e:=os.Stat(dest);if e!=nil{return e};if !st.IsDir(){return errors.New("not a directory")};return nil
}
func setup(t *testing.T)Store{
 t.Helper();id:=Identity{Core:"core-123",Target:"target-123",Cluster:"cluster-123"}
 return Store{Directory:filepath.Join(t.TempDir(),"private"),Identity:id,Client:fakeSource{data:"etcd-snapshot-bytes",cluster:id.Cluster}}
}
func TestSnapshotVerifyResume(t *testing.T){
 s:=setup(t);ctx:=context.Background()
 a,e:=s.CreateSnapshot(ctx);if e!=nil{t.Fatal(e)}
 b,e:=s.CreateSnapshot(ctx);if e!=nil||a!=b{t.Fatalf("resume changed verified backup: %v",e)}
 if _,e=s.VerifySnapshot(ctx);e!=nil{t.Fatal(e)}
 for _,name:=range []string{"etcd.snapshot","etcd.metadata.json"}{
 st,e:=os.Lstat(filepath.Join(s.Directory,name));if e!=nil{t.Fatal(e)}
 if st.Mode().Perm()!=0600{t.Fatal("unsafe permissions")}
 }
}
func TestSnapshotRejectsForeignAndTampering(t *testing.T){
 s:=setup(t);ctx:=context.Background();if _,e:=s.CreateSnapshot(ctx);e!=nil{t.Fatal(e)}
 s.Identity.Cluster="foreign-cluster"
 if _,e:=s.VerifySnapshot(ctx);e==nil{t.Fatal("foreign snapshot accepted")}
 s.Identity.Cluster="cluster-123"
 f,e:=os.OpenFile(filepath.Join(s.Directory,"etcd.snapshot"),os.O_APPEND|os.O_WRONLY,0600);if e!=nil{t.Fatal(e)}
 if _,e=f.WriteString("tampered");e!=nil{t.Fatal(e)};f.Close()
 if _,e=s.VerifySnapshot(ctx);e==nil{t.Fatal("tampered snapshot accepted")}
}
func TestSnapshotRejectsSymlinkAndWeakPermissions(t *testing.T){
 s:=setup(t);ctx:=context.Background();if _,e:=s.CreateSnapshot(ctx);e!=nil{t.Fatal(e)}
 p:=filepath.Join(s.Directory,"etcd.snapshot")
 if e:=os.Chmod(p,0644);e!=nil{t.Fatal(e)}
 if _,e:=s.VerifySnapshot(ctx);e==nil{t.Fatal("world-readable snapshot accepted")}
 if e:=os.Chmod(p,0600);e!=nil{t.Fatal(e)}
 if e:=os.Rename(p,p+".old");e!=nil{t.Fatal(e)}
 if e:=os.Symlink(p+".old",p);e!=nil{t.Fatal(e)}
 if _,e:=s.VerifySnapshot(ctx);e==nil{t.Fatal("symlink accepted")}
}
func TestSnapshotCleansPartialStream(t *testing.T){
 s:=setup(t);s.Client=fakeSource{err:errors.New("network disconnected")}
 if _,e:=s.CreateSnapshot(context.Background());e==nil{t.Fatal("stream error ignored")}
 names,e:=os.ReadDir(s.Directory);if e!=nil{t.Fatal(e)}
 if len(names)!=0{t.Fatalf("incomplete artifacts remain: %v",names)}
}
func TestRestoreIsolatedContract(t *testing.T){
 s:=setup(t);ctx:=context.Background()
 if _,e:=s.CreateSnapshot(ctx);e!=nil{t.Fatal(e)}
 dst:=filepath.Join(t.TempDir(),"isolated")
 plan,e:=s.PrepareRestore(ctx,dst);if e!=nil{t.Fatal(e)}
 restored:=&fakeRestore{}
 if e:=s.Restore(ctx,plan,restored);e!=nil{t.Fatal(e)}
 if e:=s.VerifyRecovery(ctx,plan,restored);e!=nil{t.Fatal(e)}
 if restored.called!=1{t.Fatal("restore not invoked")}
 if e:=s.Restore(ctx,plan,restored);e==nil{t.Fatal("existing target overwritten")}
}
func TestRestoreRejectsChangedPlan(t *testing.T){
 s:=setup(t);ctx:=context.Background();if _,e:=s.CreateSnapshot(ctx);e!=nil{t.Fatal(e)}
 plan,e:=s.PrepareRestore(ctx,filepath.Join(t.TempDir(),"target"));if e!=nil{t.Fatal(e)}
 plan.SHA256=strings.Repeat("0",64)
 client:=&fakeRestore{}
 if e=s.Restore(ctx,plan,client);e==nil{t.Fatal("tampered plan accepted")}
 if client.called!=0{t.Fatal("unsafe restore invoked")}
}
