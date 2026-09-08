package openbao

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestDynamicApplicationSecretLifecycleRealOpenBao(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("real OpenBao dynamic secret integration test runs in GitHub Actions")
	}

	root := t.TempDir()
	files, err := bhruntime.EnsureFiles(filepath.Join(root, ".baseharbor", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_POSTGRES_DB=baseharbor\nBASEHARBOR_POSTGRES_USER=baseharbor\nBASEHARBOR_POSTGRES_PASSWORD=dynamic-integration\nBASEHARBOR_POSTGRES_PORT=35432\nBASEHARBOR_OPENBAO_PORT=38200\n"), 0o600); err != nil {
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
	if err := Bootstrap(ctx, compose, files, filepath.Join(root, "recovery", "openbao.json")); err != nil {
		t.Fatal(err)
	}

	identity := ApplicationIdentity{Name: "dynamic", Environment: "dev"}
	credentialsPath := filepath.Join(root, "apps", "dynamic", "runtime", "openbao.env")
	if err := EnsureApplicationScope(ctx, compose, files, identity, credentialsPath); err != nil {
		t.Fatal(err)
	}

	const key = "dyn-0123456789abcdef0123456789abcdef"
	first := []byte("provider-key-v1")
	second := []byte("provider-key-v2")
	if err := SetApplicationSecret(ctx, compose, files, identity, credentialsPath, key, first); err != nil {
		t.Fatalf("create dynamic secret: %v", err)
	}
	value, err := GetApplicationSecret(ctx, compose, files, identity, credentialsPath, key)
	if err != nil || !bytes.Equal(value, first) {
		t.Fatalf("read dynamic secret: value=%q err=%v", value, err)
	}
	if err := SetApplicationSecret(ctx, compose, files, identity, credentialsPath, key, second); err != nil {
		t.Fatalf("rotate dynamic secret: %v", err)
	}
	value, err = GetApplicationSecret(ctx, compose, files, identity, credentialsPath, key)
	if err != nil || !bytes.Equal(value, second) {
		t.Fatalf("read rotated dynamic secret: value=%q err=%v", value, err)
	}
	if err := DeleteApplicationSecret(ctx, compose, files, identity, credentialsPath, key); err != nil {
		t.Fatalf("delete dynamic secret: %v", err)
	}
	if _, err := GetApplicationSecret(ctx, compose, files, identity, credentialsPath, key); !errors.Is(err, ErrApplicationSecretNotFound) {
		t.Fatalf("deleted dynamic secret read error = %v, want ErrApplicationSecretNotFound", err)
	}
}
