package openbao

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type fakeExecutor struct {
	initialized bool
	sealed      bool
	args        []string
}

func (f *fakeExecutor) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.args = append(f.args, joined)
	if strings.Contains(joined, "bao status -format=json") {
		return statusJSON(f.initialized, f.sealed), nil
	}
	if strings.Contains(joined, "bao operator init") {
		f.initialized = true
		f.sealed = true
		return "{\"unseal_keys_b64\":[\"unseal-secret\"],\"root_token\":\"root-secret\"}", nil
	}
	return "", nil
}

func (f *fakeExecutor) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.args = append(f.args, joined)
	if strings.Contains(joined, "sys/unseal") {
		var payload struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(input, &payload); err != nil || payload.Key != "unseal-secret" {
			return "", ErrInvalidRecoveryFile
		}
		f.sealed = false
		return "{\"sealed\":false}", nil
	}
	if strings.Contains(joined, "role-id") {
		return "{\"data\":{\"role_id\":\"role-id\"}}", nil
	}
	if strings.Contains(joined, "secret-id") {
		return "{\"data\":{\"secret_id\":\"manager-secret\"}}", nil
	}
	if strings.Contains(joined, "auth/approle/login") {
		return "{\"auth\":{\"client_token\":\"manager-token\"}}", nil
	}
	if strings.Contains(joined, "kv get") {
		return "ok\n", nil
	}
	return "", nil
}

func statusJSON(initialized, sealed bool) string {
	return fmt.Sprintf("{\"initialized\":%t,\"sealed\":%t,\"version\":\"2.6.2\"}", initialized, sealed)
}

func TestBootstrapKeepsRootAndUnsealSecretsOutOfCommandArguments(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(runtimeDir, "compose.yaml"),
		Env:     filepath.Join(runtimeDir, "runtime.env"),
	}
	recovery := filepath.Join(dir, "openbao-recovery.json")
	executor := &fakeExecutor{}
	if err := Bootstrap(context.Background(), executor, files, recovery); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	for _, args := range executor.args {
		for _, secret := range []string{"root-secret", "unseal-secret", "manager-secret", "manager-token"} {
			if strings.Contains(args, secret) {
				t.Fatalf("secret %q leaked into command arguments %q", secret, args)
			}
		}
	}

	recoveryData, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recoveryData), "root-secret") || strings.Contains(string(recoveryData), "manager-secret") {
		t.Fatal("recovery file contains non-recovery credentials")
	}
	if !strings.Contains(string(recoveryData), "unseal-secret") {
		t.Fatal("recovery file is missing the unseal material")
	}
	assertOwnerOnly(t, recovery)

	adminPath := AdminCredentialsPath(files)
	adminData, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(adminData), "root-secret") || strings.Contains(string(adminData), "unseal-secret") {
		t.Fatal("manager credential file contains root or unseal material")
	}
	if !strings.Contains(string(adminData), "OPENBAO_SECRET_ID=manager-secret") {
		t.Fatal("manager credential file is missing the AppRole SecretID")
	}
	assertOwnerOnly(t, adminPath)
}

func TestBootstrapRejectsRecoveryFileInsideBaseHarborState(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{Compose: filepath.Join(runtimeDir, "compose.yaml"), Env: filepath.Join(runtimeDir, "runtime.env")}
	executor := &fakeExecutor{}
	err := Bootstrap(context.Background(), executor, files, filepath.Join(dir, ".baseharbor", "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside .baseharbor") {
		t.Fatalf("expected recovery path rejection, got %v", err)
	}
	if executor.initialized {
		t.Fatal("OpenBao was initialized despite invalid recovery path")
	}
}

func TestBootstrapRejectsRecoveryDirectorySymlinkedIntoBaseHarborState(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "recovery-link")
	if err := os.Symlink(filepath.Join(dir, ".baseharbor"), link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	files := bhruntime.Files{Compose: filepath.Join(runtimeDir, "compose.yaml"), Env: filepath.Join(runtimeDir, "runtime.env")}
	executor := &fakeExecutor{}
	err := Bootstrap(context.Background(), executor, files, filepath.Join(link, "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside .baseharbor") {
		t.Fatalf("expected symlinked recovery path rejection, got %v", err)
	}
	if executor.initialized {
		t.Fatal("OpenBao was initialized despite symlinked recovery path")
	}
}

func TestLoadAdminCredentialsRejectsBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	files := bhruntime.Files{Env: filepath.Join(dir, "runtime.env")}
	path := AdminCredentialsPath(files)
	if err := os.WriteFile(path, []byte("OPENBAO_ROLE_ID=role\nOPENBAO_SECRET_ID=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdminCredentials(files); err == nil {
		t.Fatal("expected insecure permissions to be rejected")
	}
}

func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("%s is accessible by group or others: %o", path, info.Mode().Perm())
	}
}
