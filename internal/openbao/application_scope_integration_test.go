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
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv put -mount=baseharbor apps/alpha/dev marker=preserved`); err != nil {
		t.Fatal(err)
	}
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv get -mount=baseharbor apps/beta/dev`); err == nil {
		t.Fatal("alpha application identity could read beta secret scope")
	}
	if _, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv put -mount=baseharbor apps/beta/dev escaped=true`); err == nil {
		t.Fatal("alpha application identity could write beta secret scope")
	}

	if err := InspectApplicationScope(ctx, compose, files, alpha, alphaCredPath); err != nil {
		t.Fatalf("read-only scope inspection failed: %v", err)
	}
	out, err := execWithToken(ctx, compose, files, alphaToken, `exec bao kv get -field=marker -mount=baseharbor apps/alpha/dev`)
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
	if err := DestroyVerifiedApplicationScope(ctx, compose, files, alpha); err != nil {
		t.Fatalf("destroy alpha: %v", err)
	}
	if err := DestroyVerifiedApplicationScope(ctx, compose, files, beta); err != nil {
		t.Fatalf("destroy beta: %v", err)
	}
}
