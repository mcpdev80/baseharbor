package main

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "strings"
    "testing"

    "github.com/mcpdev80/baseharbor/internal/application"
    "github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestGuidedReadContextRefusesMissingAndWrongTarget(t *testing.T) {
    for _, tc := range []struct {
        name string
        resolved resolvedApplication
        kind string
    }{
        {"missing", resolvedApplication{}, "missing"},
        {"wrong target", resolvedApplication{
            Target:deployment.ResolvedTarget{Name:"test"},
            DeploymentIdentity:deployment.DeploymentIdentity{Target:"prod", ApplicationID:"owned"},
            Manifest:application.Manifest{Name:"app", ApplicationID:"owned"},
        }, "wrong_target"},
        {"unverified id", resolvedApplication{
            Target:deployment.ResolvedTarget{Name:"test"},
            DeploymentIdentity:deployment.DeploymentIdentity{Target:"test", ApplicationID:"different"},
            Manifest:application.Manifest{Name:"app", ApplicationID:"owned"},
        }, "unverifiable"},
    } {
        t.Run(tc.name,func(t *testing.T) {
            _,err:=readGuidedApplicationContext(context.Background(),tc.resolved)
            var typed *GuidedReadError
            if !errors.As(err,&typed) || typed.Kind!=tc.kind { t.Fatalf("error = %v, expected %s",err,tc.kind) }
        })
    }
}

func TestGuidedReadNewAndEditDoNotWriteState(t *testing.T) {
    for _, edit := range []bool{false, true} {
        t.Run(map[bool]string{false:"new",true:"edit"}[edit],func(t *testing.T) {
            r:=corelessResolvedFixture(t)
            if edit {
                r.DeploymentIdentity=deployment.DeploymentIdentity{
                    Target:r.Target.Name,ApplicationID:r.Manifest.ApplicationID,
                    DeploymentID:"existing"}
            }
            before, statErr:=os.Stat(r.TargetStateRoot)
            state,err:=readGuidedApplicationContext(context.Background(),r)
            if err!=nil {t.Fatalf("actual read: %v",err)}
            if state.Target!=r.Target.Name || state.Name!=r.Manifest.Name ||
                state.SnapshotRevision=="" || state.PlanRevision=="" {
                t.Fatalf("incomplete authoritative projection: %+v",state)
            }
            after, afterErr:=os.Stat(r.TargetStateRoot)
            if (statErr==nil)!=(afterErr==nil) || (before!=nil && after!=nil && !os.SameFile(before,after)) {
                t.Fatalf("read modified target root: %v / %v",statErr,afterErr)
            }
            payload,err:=json.Marshal(state)
            if err!=nil {t.Fatal(err)}
            if strings.Contains(string(payload),"secret_value") || strings.Contains(string(payload),"password") {
                t.Fatal("secret material leaked")
            }
        })
    }
}

func TestGuidedRevisionsTrackInputs(t *testing.T) {
    a,err:=guidedRevision(struct{Target string; Version int}{"test",1})
    if err!=nil || len(a)!=64 { t.Fatalf("revision: %q %v",a,err) }
    same,err:=guidedRevision(struct{Target string; Version int}{"test",1})
    if err!=nil || same!=a { t.Fatalf("revision is unstable: %v",err) }
    changed,err:=guidedRevision(struct{Target string; Version int}{"test",2})
    if err!=nil || changed==a { t.Fatalf("changed input retained revision: %v",err) }
}

func TestGuidedPublicShapeDoesNotExportRawSecretsOrManifests(t *testing.T) {
    state:=GuidedReadContext{ApplicationID:"owned",Name:"app",Target:"test",SnapshotRevision:"rev"}
    encoded,err:=json.Marshal(state)
    if err!=nil {t.Fatal(err)}
    for _,forbidden:=range []string{"manifest","raw_intent","credentials","secret_value","facts","registry"} {
        if strings.Contains(string(encoded),"\""+forbidden+"\"") {t.Fatalf("public field %q leaked",forbidden)}
    }
}
