package application
import ("os";"path/filepath";"testing")
// ROLLE: REVIEW. Unchanged placement must preserve current protected credential material.
func TestCMF23UnchangedPlacementPreservesRecoveredCredentials(t *testing.T) {
 m:=dedicatedSQLManifest(t,"credential-recovery")
 p:=filepath.Join(t.TempDir(),"runtime.env")
 if err:=ensureRuntimeEnv(p,m);err!=nil{t.Fatal(err)}
 v:=readPlacementRuntimeEnv(t,p)
 replacement,err:=randomApplicationSecret(32);if err!=nil{t.Fatal(err)}
 v[postgresRuntimeKey(defaultServiceInstance,"PASSWORD")]=replacement
 if err:=writeRuntimeEnv(p,m,v);err!=nil{t.Fatal(err)}
 if err:=ensureRuntimeEnv(p,m);err!=nil{t.Fatal(err)}
 after:=readPlacementRuntimeEnv(t,p)
 if after[postgresRuntimeKey(defaultServiceInstance,"PASSWORD")]!=replacement {t.Fatal("unchanged dedicated placement silently overwrote current protected credential with older owner slot")}
}
// ROLLE: REVIEW. Losing native recovery material during shared use must not re-label shared credentials as native.
func TestCMF23MissingNativeOwnerCannotCaptureSharedSecret(t *testing.T) {
 m:=dedicatedSQLManifest(t,"missing-native-owner")
 p:=filepath.Join(t.TempDir(),"runtime.env")
 if err:=ensureRuntimeEnv(p,m);err!=nil{t.Fatal(err)}
 original:=readPlacementRuntimeEnv(t,p)
 shared:=sharedSQLManifest(t,"missing-native-owner")
 if err:=ensureRuntimeEnv(p,shared);err!=nil{t.Fatal(err)}
 simulateSharedBackendProjection(t,p,shared,defaultServiceInstance)
 v:=readPlacementRuntimeEnv(t,p)
 delete(v,postgresRuntimeKey(defaultServiceInstance,"APPLICATION_SECRET"))
 if err:=writeRuntimeEnv(p,shared,v);err!=nil{t.Fatal(err)}
 before,err:=os.ReadFile(p);if err!=nil{t.Fatal(err)}
 err=ensureRuntimeEnv(p,m)
 if err!=nil {after,_:=os.ReadFile(p);if string(after)!=string(before){t.Fatal("failed recovery rewrote protected owner")};return}
 after:=readPlacementRuntimeEnv(t,p)
 if after[postgresRuntimeKey(defaultServiceInstance,"PASSWORD")]!=original[postgresRuntimeKey(defaultServiceInstance,"PASSWORD")] {t.Fatal("missing native secret silently replaced with shared-era secret; retained dedicated volume cannot authenticate")}
}
