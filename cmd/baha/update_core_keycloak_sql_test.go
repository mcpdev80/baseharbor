package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func TestNativeKeycloakSQLVerificationUsesAppRoleAndTrustedTLS(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "runtime.env")
	const password = "keycloak-secret-never-in-argv"
	values := "BASEHARBOR_KEYCLOAK_DB_USER=keycloak_app\nBASEHARBOR_KEYCLOAK_DB_PASSWORD=" + password + "\nBASEHARBOR_KEYCLOAK_DB_NAME=keycloak\n"
	if err := os.WriteFile(env, []byte(values), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &openBaoSQLProbeRuntime{}
	ops := coreNativeRuntimeOps{runtime: runtime, identity: identityprovider.KeycloakFiles{Project: "identity", Compose: filepath.Join(dir, "compose.yaml"), Env: env}}
	if err := ops.verifyKeycloakBackingSQL(context.Background()); err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(runtime.args, " ")
	if runtime.service != "keycloak-db" || !strings.Contains(argv, "verify-full") || !strings.Contains(argv, "keycloak_app") || !strings.Contains(argv, "public.realm") || !strings.Contains(argv, "public.client") || !strings.Contains(runtime.input, password) {
		t.Fatalf("Keycloak owned TLS SQL probe missing: service=%s args=%v", runtime.service, runtime.args)
	}
	if strings.Contains(argv, password) {
		t.Fatal("Keycloak database password leaked through process argv")
	}
	runtime.fail = true
	if err := ops.verifyKeycloakBackingSQL(context.Background()); err == nil {
		t.Fatal("Keycloak SQL auth failure accepted")
	}
	runtime.fail = false
	if err := os.WriteFile(env, []byte("BASEHARBOR_KEYCLOAK_DB_USER=keycloak\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ops.verifyKeycloakBackingSQL(context.Background()); err == nil {
		t.Fatal("missing protected Keycloak SQL secrets accepted")
	}
}
