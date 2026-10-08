package coreupdate

import "testing"

func TestPinnedReleaseCatalog(t *testing.T) {
 manifest,err:=LoadRelease("v0.4.24")
 if err!=nil {t.Fatal(err)}
 if len(manifest.Providers)!=3 || len(manifest.Backing)!=2 {
  t.Fatalf("incomplete immutable provider catalog: %+v",manifest)
 }
 for _,item:=range manifest.Providers {
  if !validDigest(item.Digest) {t.Fatalf("invalid digest for %s",item.Kind)}
 }
 for _,bad:=range []string{"latest","0.4.23","../0.4.24","v0.4.24-extra"} {
  if _,err:=LoadRelease(bad);err==nil {t.Fatalf("accepted missing/invalid release %q",bad)}
 }
}
