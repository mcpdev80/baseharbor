package openbao

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestApplicationScopesRealOpenBaoIsolation(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("real OpenBao integration test runs in GitHub Actions")
	}

	root := t.TempDir()
	runtimeDir := filepath.Join(root, ".baseharbor", "runtime")
	files, err := bhruntime.EnsureFiles(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=integration-only\nBASEHARBOR_POSTGRES_PORT=25432\nBASEHARBOR_OPENBAO_PORT=28200\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.UpProject(ctx, projectName, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = compose.DestroyProject(cleanupCtx, projectName, files.Compose, files.Env)
	}()

	var state State
	for ctx.Err() == nil {
		state, err = Inspect(ctx, compose, files)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("OpenBao did not become reachable: %v", err)
	}
	if state.Initialized {
		t.Fatal("integration OpenBao unexpectedly started initialized")
	}

	recoveryPath := filepath.Join(root, "recovery", "openbao.json")
	if err := Bootstrap(ctx, compose, files, recoveryPath); err != nil {
		t.Fatal(err)
	}

	alpha := ApplicationIdentity{Name: "alpha", Environment: "dev"}
	beta := ApplicationIdentity{Name: "beta", Environment: "dev"}
	alphaCredPath := filepath.Join(root, "apps", "alpha", "runtime", "openbao.env")
	betaCredPath := filepath.Join(root, "apps", "beta", "runtime", "openbao.env")
	if err := CheckApplicationProvisioning(ctx, compose, files, alpha); err != nil {
		t.Fatalf("manager provisioning preflight failed: %v", err)
	}
	if err := EnsureApplicationScope(ctx, compose, files, alpha, alphaCredPath); err != nil {
		t.Fatalf("provision alpha: %v", err)
	}
	if err := EnsureApplicationScope(ctx, compose, files, beta, betaCredPath); err != nil {
		t.Fatalf("provision beta: %v", err)
	}

	alphaCreds, err := loadApplicationCredentials(alphaCredPath)
	if err != nil {
		t.Fatal(err)
	}
	alphaToken, err := loginApplication(ctx, compose, files, alphaCreds)
	if err != nil {
		t.Fatal(err)
	}
	betaCreds, err := loadApplicationCredentials(betaCredPath)
	if err != nil {
		t.Fatal(err)
	}
	betaToken, err := loginApplication(ctx, compose, files, betaCreds)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv put -mount=baseharbor apps/alpha/dev marker=preserved`); err != nil {
		t.Fatal(err)
	}
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv get -mount=baseharbor apps/beta/dev`); err == nil {
		t.Fatal("alpha application identity could read beta secret scope")
	}
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv put -mount=baseharbor apps/beta/dev escaped=true`); err == nil {
		t.Fatal("alpha application identity could write beta secret scope")
	}

	statuses, err := InspectRequiredApplicationSecrets(ctx, compose, files, alpha, alphaCredPath, []string{"API_TOKEN"})
	if err != nil {
		t.Fatalf("inspect missing required secret: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Present || statuses[0].Usable || RequireApplicationSecrets(statuses) == nil {
		t.Fatalf("missing required secret was not reported correctly: %#v", statuses)
	}

	if err := SetApplicationSecret(ctx, compose, files, alpha, alphaCredPath, "API_TOKEN", []byte("top-secret")); err != nil {
		t.Fatalf("set alpha application secret: %v", err)
	}
	statuses, err = InspectRequiredApplicationSecrets(ctx, compose, files, alpha, alphaCredPath, []string{"API_TOKEN"})
	if err != nil {
		t.Fatalf("inspect configured required secret: %v", err)
	}
	if len(statuses) != 1 || !statuses[0].Present || !statuses[0].Usable || RequireApplicationSecrets(statuses) != nil {
		t.Fatalf("configured required secret was not reported usable: %#v", statuses)
	}

	keys, err := ListApplicationSecretKeys(ctx, compose, files, alpha, alphaCredPath)
	if err != nil {
		t.Fatalf("list alpha application secrets: %v", err)
	}
	if len(keys) != 1 || keys[0] != "API_TOKEN" {
		t.Fatalf("unexpected application secret keys: %#v", keys)
	}
	out, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv get -field=value -mount=baseharbor apps/alpha/dev/API_TOKEN`)
	if err != nil || strings.TrimSpace(out) != "top-secret" {
		t.Fatal("alpha application identity could not read its managed secret value")
	}
	if _, err := execWithToken(ctx, compose, files, betaToken, `exec bao kv get -mount=baseharbor apps/alpha/dev/API_TOKEN`); err == nil {
		t.Fatal("beta application identity could read alpha managed secret value")
	}
	if _, err := execWithToken(ctx, compose, files, betaToken, `exec bao kv put -mount=baseharbor apps/alpha/dev/API_TOKEN value=escaped`); err == nil {
		t.Fatal("beta application identity could overwrite alpha managed secret value")
	}
	if err := DeleteApplicationSecret(ctx, compose, files, alpha, alphaCredPath, "API_TOKEN"); err != nil {
		t.Fatalf("delete alpha application secret: %v", err)
	}
	statuses, err = InspectRequiredApplicationSecrets(ctx, compose, files, alpha, alphaCredPath, []string{"API_TOKEN"})
	if err != nil {
		t.Fatalf("inspect deleted required secret: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Present || statuses[0].Usable || RequireApplicationSecrets(statuses) == nil {
		t.Fatalf("deleted required secret remained ready: %#v", statuses)
	}

	keys, err = ListApplicationSecretKeys(ctx, compose, files, alpha, alphaCredPath)
	if err != nil {
		t.Fatalf("list alpha application secrets after delete: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("deleted secret still listed: %#v", keys)
	}
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv metadata get -mount=baseharbor apps/alpha/dev/API_TOKEN`); err == nil {
		t.Fatal("deleted secret metadata or historical versions remain accessible")
	}

	if err := InspectApplicationScope(ctx, compose, files, alpha, alphaCredPath); err != nil {
		t.Fatalf("read-only scope inspection failed: %v", err)
	}
	out, err = execWithToken(ctx, compose, files, alphaToken, `exec bao kv get -field=marker -mount=baseharbor apps/alpha/dev`)
	if err != nil || strings.TrimSpace(out) != "preserved" {
		t.Fatal("read-only scope inspection changed application secret data")
	}

	managerCredentials, err := LoadAdminCredentials(files)
	if err != nil {
		t.Fatal(err)
	}
	managerToken, err := loginManager(ctx, compose, files, managerCredentials)
	if err != nil {
		t.Fatal(err)
	}
	const tamperedPolicy = `path "baseharbor/data/apps/alpha/dev" {
  capabilities = ["read"]
}
`
	policyScript := `tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp"
bao policy write baseharbor-app-alpha-dev "$tmp" >/dev/null`
	if _, err := execWithTokenPayload(ctx, compose, files, managerToken, policyScript, tamperedPolicy); err != nil {
		t.Fatal(err)
	}
	if err := DestroyVerifiedApplicationScope(ctx, compose, files, alpha); err == nil {
		t.Fatal("destroy accepted a tampered application policy")
	}
	out, err = execWithToken(ctx, compose, files, managerToken, `exec bao kv get -field=marker -mount=baseharbor apps/alpha/dev`)
	if err != nil || strings.TrimSpace(out) != "preserved" {
		t.Fatal("failed destroy preflight changed application secret data")
	}

	if err := EnsureApplicationScope(ctx, compose, files, alpha, alphaCredPath); err != nil {
		t.Fatalf("reconcile alpha policy: %v", err)
	}
	if err := SetApplicationSecret(ctx, compose, files, alpha, alphaCredPath, "DESTROY_ME", []byte("destroyed-with-app")); err != nil {
		t.Fatalf("set pre-destroy application secret: %v", err)
	}
	if err := DestroyVerifiedApplicationScope(ctx, compose, files, alpha); err != nil {
		t.Fatalf("destroy alpha: %v", err)
	}
	if _, err := execWithToken(ctx, compose, files, managerToken, `exec bao kv metadata get -mount=baseharbor apps/alpha/dev/DESTROY_ME`); err == nil {
		t.Fatal("application destroy left managed secret metadata behind")
	}
	if _, err := execWithToken(ctx, compose, files, managerToken, `exec bao kv metadata get -mount=baseharbor apps/alpha/dev/_baseharbor`); err == nil {
		t.Fatal("application destroy left managed namespace marker behind")
	}
	if err := DestroyVerifiedApplicationScope(ctx, compose, files, beta); err != nil {
		t.Fatalf("destroy beta: %v", err)
	}
}
