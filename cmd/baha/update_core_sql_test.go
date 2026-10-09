package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type openBaoSQLProbeRuntime struct {
	bhruntime.RuntimeProvider
	input   string
	service string
	args    []string
	fail    bool
}

func (r *openBaoSQLProbeRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, service string, args ...string) (string, error) {
	r.input = string(input)
	r.service = service
	r.args = append([]string(nil), args...)
	if r.fail {
		return "", errors.New("SQL authentication rejected")
	}
	return "1", nil
}

func TestNativeOpenBaoSQLVerificationProtectsCredentials(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "runtime.env")
	const password = "strong-sql-secret-not-in-argv"
	content := strings.Join([]string{
		"BASEHARBOR_POSTGRES_USER=core",
		"BASEHARBOR_POSTGRES_PASSWORD=pw",
		"BASEHARBOR_POSTGRES_INTERNAL_USER=postgres",
		"BASEHARBOR_POSTGRES_INTERNAL_PASSWORD=internal",
		"BASEHARBOR_POSTGRES_REPLICATION_USER=replication",
		"BASEHARBOR_POSTGRES_REPLICATION_PASSWORD=replication-pw",
		"BASEHARBOR_OPENBAO_DB_USER=openbao",
		"BASEHARBOR_OPENBAO_DB_PASSWORD=" + password,
	}, "\n") + "\n"
	if err := os.WriteFile(env, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &openBaoSQLProbeRuntime{}
	ops := coreNativeRuntimeOps{runtime: runtime, core: bhruntime.Files{Project: "core", Compose: filepath.Join(dir, "compose.yaml"), Env: env}}
	if err := ops.verifyOpenBaoBackingSQL(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.service != "postgres-admin" || !strings.Contains(strings.Join(runtime.args, " "), "openbao") || !strings.Contains(runtime.input, password) {
		t.Fatalf("OpenBao protected SQL probe missing: service=%s args=%v", runtime.service, runtime.args)
	}
	if strings.Contains(strings.Join(runtime.args, " "), password) {
		t.Fatal("SQL password leaked in process arguments")
	}
	runtime.fail = true
	if err := ops.verifyOpenBaoBackingSQL(context.Background()); err == nil {
		t.Fatal("failed OpenBao SQL authentication accepted")
	}
}

type nativeAdapterImageRuntime struct {
	bhruntime.RuntimeProvider
	digest string
}

func (r *nativeAdapterImageRuntime) ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error) {
	return bhruntime.ImageIdentity{Reference: "quay.io/keycloak/keycloak:26.8.1", Digest: r.digest}, nil
}
func TestNativeProviderAdapterRejectsUnknownImageBeforeSemanticProbe(t *testing.T) {
	d := coreupdate.Delta{
		Installed: coreupdate.Realization{Kind: coreupdate.Identity, Instance: "keycloak-1", Digest: "sha256:" + strings.Repeat("a", 64), Version: "26.8.0"},
		Desired:   coreupdate.Desired{Kind: coreupdate.Identity, Digest: "sha256:" + strings.Repeat("b", 64), Version: "26.8.1", Image: "quay.io/keycloak/keycloak:26.8.1"},
	}
	rt := &nativeAdapterImageRuntime{digest: "sha256:" + strings.Repeat("c", 64)}
	ops := coreNativeRuntimeOps{runtime: rt, identity: identityprovider.KeycloakFiles{Project: "managed-keycloak"}}
	if err := ops.verifyBoundProviderSemantics(context.Background(), d); err == nil {
		t.Fatal("unknown provider runtime digest accepted")
	}
	rt.digest = d.Installed.Digest
	if err := ops.verifyBoundProviderSemantics(context.Background(), d); err != nil {
		t.Fatalf("unmutated original image rejected for recovery: %v", err)
	}
}

type nativeKeycloakAdmissionRuntime struct {
 bhruntime.RuntimeProvider
 containers []bhruntime.RuntimeContainer
 image bhruntime.ImageIdentity
}
func (r *nativeKeycloakAdmissionRuntime) ListRuntimeContainers(context.Context)([]bhruntime.RuntimeContainer,error){
 return r.containers,nil
}
func (r *nativeKeycloakAdmissionRuntime) ProjectServiceImageIdentity(context.Context,string,string)(bhruntime.ImageIdentity,error){
 return r.image,nil
}
func TestNativeKeycloakAdapterUpgradeAdmission(t *testing.T){
 r:=&nativeKeycloakAdmissionRuntime{
  containers:[]bhruntime.RuntimeContainer{{Project:"identity-project",Service:"keycloak-1",Running:true,Health:"healthy"}},
  image:bhruntime.ImageIdentity{Reference:"quay.io/keycloak/keycloak:26.8.0",Digest:"sha256:"+strings.Repeat("a",64)},
 }
 ops:=coreNativeRuntimeOps{runtime:r,identity:identityprovider.KeycloakFiles{Project:"identity-project"}}
 d:=coreupdate.Delta{
  Installed:coreupdate.Realization{Kind:coreupdate.Identity,Instance:"keycloak-1",Version:"26.8.0",Digest:r.image.Digest},
  Desired:coreupdate.Desired{Kind:coreupdate.Identity,Version:"26.8.1",Image:"quay.io/keycloak/keycloak:26.8.1",Digest:"sha256:"+strings.Repeat("b",64)},
  Classification:coreupdate.BackupRequired,
 }
 if err:=ops.admitNativeProviderTransition(context.Background(),d);err!=nil{t.Fatalf("safe Keycloak patch rejected: %v",err)}
 d.Desired.Version="26.9.0"
 if err:=ops.admitNativeProviderTransition(context.Background(),d);err==nil{t.Fatal("unreviewed Keycloak minor upgrade accepted")}
 d.Desired.Version="26.8.1"
 r.containers=append(r.containers,r.containers[0])
 if err:=ops.admitNativeProviderTransition(context.Background(),d);err==nil{t.Fatal("duplicate Keycloak member accepted")}
 r.containers=r.containers[:1]
 r.image.Reference="quay.io/keycloak/keycloak:26.7.9"
 if err:=ops.admitNativeProviderTransition(context.Background(),d);err==nil{t.Fatal("stale Keycloak version accepted")}
}
