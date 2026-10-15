package main

import (
    "context"
    "encoding/json"
    "errors"
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
