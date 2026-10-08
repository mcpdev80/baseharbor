package etcd

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
)

// EtcdctlSource invokes a separately installed trusted etcdctl and etcdutl,
// never a command inside the distroless etcd container. Credentials remain in
// protected files; only path locations are passed through the child environment.
type EtcdctlSource struct {
 Etcdctl string
 Etcdutl string
 Endpoints []string
 Identity Identity
 TLS TLSFiles
 ScratchDir string
}
func trustedBinary(path string)error{
 if path=="" || !filepath.IsAbs(path){return errors.New("trusted helper must have an absolute path")}
 st,err:=os.Lstat(path);if err!=nil{return err}
 if !st.Mode().IsRegular() || st.Mode().Perm()&0022!=0 {return errors.New("untrusted helper executable")}
 return nil
}
func (s EtcdctlSource) validate()error{
 if err:=s.Identity.Validate();err!=nil{return err}
 if err:=trustedBinary(s.Etcdctl);err!=nil{return err}
 if err:=trustedBinary(s.Etcdutl);err!=nil{return err}
 if len(s.Endpoints)==0 || len(s.Endpoints)>9{return errors.New("explicit owned etcd endpoints required")}
 for _,endpoint:=range s.Endpoints {
  parsed,err:=url.Parse(endpoint)
  if err!=nil || parsed.Scheme!="https" || parsed.Host=="" || parsed.User!=nil || parsed.Path!="" || parsed.RawQuery!="" || parsed.Fragment!="" {
   return errors.New("etcd endpoint must be an explicit HTTPS authority")
  }
 }
 if _,err:=s.TLS.Config();err!=nil{return fmt.Errorf("etcd client mTLS unavailable: %w",err)}
 return safeDirectory(s.ScratchDir)
}
func (s EtcdctlSource) Snapshot(ctx context.Context,dest io.Writer)(SnapshotInfo,error){
 if dest==nil{return SnapshotInfo{},errors.New("snapshot stream destination required")}
 if err:=s.validate();err!=nil{return SnapshotInfo{},err}
 scratch,err:=os.MkdirTemp(s.ScratchDir,".etcdctl-*")
 if err!=nil{return SnapshotInfo{},err}
 defer os.RemoveAll(scratch)
 if err:=os.Chmod(scratch,0700);err!=nil{return SnapshotInfo{},err}
 archive:=filepath.Join(scratch,"snapshot.db")
 // Only the approved endpoints are used. No token/certificate bytes in argv.
 cmd:=exec.CommandContext(ctx,s.Etcdctl,"snapshot","save",archive)
 cmd.Env=[]string{
  "PATH=/usr/bin:/bin","ETCDCTL_API=3",
  "ETCDCTL_ENDPOINTS="+strings.Join(s.Endpoints,","),
  "ETCDCTL_CACERT="+s.TLS.CA,
  "ETCDCTL_CERT="+s.TLS.Cert,
  "ETCDCTL_KEY="+s.TLS.Key,
 }
 if err:=cmd.Run();err!=nil{return SnapshotInfo{},errors.New("etcd maintenance snapshot failed")}
 if err:=safeRegular(archive);err!=nil{return SnapshotInfo{},err}
 verify:=exec.CommandContext(ctx,s.Etcdutl,"snapshot","status",archive,"--write-out=json")
 verify.Env=[]string{"PATH=/usr/bin:/bin"}
 statusJSON,err:=verify.Output()
 if err!=nil{return SnapshotInfo{},errors.New("etcdutl snapshot status integrity check failed")}
 var status struct {
  Revision int64 `json:"revision"`
  TotalKey int64 `json:"totalKey"`
  TotalSize int64 `json:"totalSize"`
  Hash uint64 `json:"hash"`
 }
 if len(statusJSON)>8192 || json.Unmarshal(statusJSON,&status)!=nil || status.Revision<=0 || status.TotalSize<=0 {
  return SnapshotInfo{},errors.New("invalid etcdutl snapshot status")
 }
 f,err:=os.Open(archive);if err!=nil{return SnapshotInfo{},err}
 defer f.Close()
 if _,err:=io.Copy(dest,&contextReader{ctx:ctx,Reader:f});err!=nil{return SnapshotInfo{},err}
 // The owning Core must supply attested cluster ID and etcd version from
 // authenticated etcd status. Snapshot bytes alone cannot attest membership.
 return SnapshotInfo{ClusterID:s.Identity.Cluster,Version:"etcd-v3-attestation-required",Revision:status.Revision},nil
}
