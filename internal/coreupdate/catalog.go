package coreupdate

import (
 "embed"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "io/fs"
 "regexp"
 "strings"
)

//go:embed releases/*.json
var releaseCatalog embed.FS

var releaseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// BackingPin identifies durable backing databases owned by a Core reference
// provider, without misclassifying them as optional core capabilities.
type BackingPin struct {
 Role string `json:"role"`
 Version string `json:"version"`
 Image string `json:"image"`
 Digest string `json:"digest"`
}

type ReleaseManifest struct {
 Release string `json:"release"`
 Providers []Desired `json:"providers"`
 Backing []BackingPin `json:"backing"`
}

// LoadRelease resolves only digests compiled into the selected BaseHarbor
// binary. A missing release must fail; no mutable OCI tag fallback is allowed.
func LoadRelease(version string) (ReleaseManifest,error) {
 normalized:=strings.TrimPrefix(strings.TrimSpace(version),"v")
 if !releaseVersionPattern.MatchString(normalized) {
  return ReleaseManifest{},fmt.Errorf("invalid Core release version %q",version)
 }
 payload,err:=fs.ReadFile(releaseCatalog,"releases/v"+normalized+".json")
 if err!=nil {
  return ReleaseManifest{},fmt.Errorf("no immutable Core provider set for release %s: %w",normalized,err)
 }
 decoder:=json.NewDecoder(strings.NewReader(string(payload)))
 decoder.DisallowUnknownFields()
 var release ReleaseManifest
 if err:=decoder.Decode(&release);err!=nil{return ReleaseManifest{},fmt.Errorf("invalid Core release manifest: %w",err)}
 if err:=decoder.Decode(new(any));err!=io.EOF{return ReleaseManifest{},errors.New("Core release manifest contains trailing data")}
 if release.Release!=normalized{return ReleaseManifest{},errors.New("Core release manifest version mismatch")}
 if _,err:=Build(normalized,nil,release.Providers);err!=nil{return ReleaseManifest{},err}
 seen:=map[string]bool{}
 for _, backing:=range release.Backing {
  if backing.Role=="" || seen[backing.Role] || backing.Version=="" || backing.Image=="" || !validDigest(backing.Digest) {
   return ReleaseManifest{},fmt.Errorf("invalid/duplicate Core backing image role %q",backing.Role)
  }
  seen[backing.Role]=true
 }
 for _,required:=range []string{"keycloak-single-postgresql","keycloak-ha-postgresql"} {
  if !seen[required] {return ReleaseManifest{},fmt.Errorf("Core release lacks backing role %s",required)}
 }
 return release,nil
}
