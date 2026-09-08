package openbao

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestApplicationSecretBackupRestoreRealOpenBao(t *testing.T) {
	if os.Getenv("BASEHARBOR_OPENBAO_BACKUP_ACCEPTANCE") != "true" {
		t.Skip("OpenBao backup acceptance is opt-in")
	}

	root := t.TempDir()
	files, err := bhruntime.EnsureFiles(filepath.Join(root, ".baseharbor", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=backup-integration\nBASEHARBOR_POSTGRES_PORT=35442\nBASEHARBOR_OPENBAO_PORT=38210\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	if err := Bootstrap(ctx, compose, files, filepath.Join(root, "recovery", "openbao.json")); err != nil {
		t.Fatal(err)
	}

	alpha := ApplicationIdentity{Name: "backup-alpha", Environment: "dev"}
	beta := ApplicationIdentity{Name: "backup-beta", Environment: "dev"}
	alphaCreds := filepath.Join(root, "apps", alpha.Name, "runtime", "openbao.env")
	betaCreds := filepath.Join(root, "apps", beta.Name, "runtime", "openbao.env")
	if err := EnsureApplicationScope(ctx, compose, files, alpha, alphaCreds); err != nil {
		t.Fatal(err)
	}
	if err := EnsureApplicationScope(ctx, compose, files, beta, betaCreds); err != nil {
		t.Fatal(err)
	}

	const dynamicKey = "dyn-0123456789abcdef0123456789abcdef"
	if err := SetApplicationSecret(ctx, compose, files, alpha, alphaCreds, "API_TOKEN", []byte("static-v1")); err != nil {
		t.Fatal(err)
	}
	if err := SetApplicationSecret(ctx, compose, files, alpha, alphaCreds, dynamicKey, []byte("dynamic-v1")); err != nil {
		t.Fatal(err)
	}
	backup, err := ExportApplicationSecrets(ctx, compose, files, alpha, alphaCreds)
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.Secrets) != 2 {
		t.Fatalf("exported secrets = %d, want 2", len(backup.Secrets))
	}
	oldCredentials, err := loadApplicationCredentials(alphaCreds)
	if err != nil {
		t.Fatal(err)
	}

	if err := RestoreApplicationSecrets(ctx, compose, files, beta, betaCreds, backup); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected cross-application restore rejection, got %v", err)
	}
	betaKeys, err := ListApplicationSecretKeys(ctx, compose, files, beta, betaCreds)
	if err != nil {
		t.Fatal(err)
	}
	if len(betaKeys) != 0 {
		t.Fatalf("cross-application restore mutated beta: %v", betaKeys)
	}

	if err := DestroyApplicationScope(ctx, compose, files, alpha); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alphaCreds); err != nil {
		t.Fatal(err)
	}
	if err := EnsureApplicationScope(ctx, compose, files, alpha, alphaCreds); err != nil {
		t.Fatal(err)
	}
	newCredentials, err := loadApplicationCredentials(alphaCreds)
	if err != nil {
		t.Fatal(err)
	}
	if newCredentials.SecretID == oldCredentials.SecretID {
		t.Fatal("application SecretID was reused across scope recreation")
	}
	if err := RestoreApplicationSecrets(ctx, compose, files, alpha, alphaCreds, backup); err != nil {
		t.Fatal(err)
	}

	staticValue, err := GetApplicationSecret(ctx, compose, files, alpha, alphaCreds, "API_TOKEN")
	if err != nil || !bytes.Equal(staticValue, []byte("static-v1")) {
		t.Fatalf("restored static secret = %q err=%v", staticValue, err)
	}
	dynamicValue, err := GetApplicationSecret(ctx, compose, files, alpha, alphaCreds, dynamicKey)
	if err != nil || !bytes.Equal(dynamicValue, []byte("dynamic-v1")) {
		t.Fatalf("restored dynamic secret = %q err=%v", dynamicValue, err)
	}
	keys, err := ListApplicationSecretKeys(ctx, compose, files, alpha, alphaCreds)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "API_TOKEN" || keys[1] != dynamicKey {
		t.Fatalf("restored keys = %v", keys)
	}
}
