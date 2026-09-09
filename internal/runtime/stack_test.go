package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureFilesCreatesProtectedRuntimeState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{files.Compose, files.Env} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", path, info.Mode().Perm())
		}
	}

	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "BASEHARBOR_POSTGRES_PASSWORD=") {
		t.Fatal("runtime environment does not contain postgres password")
	}
	if strings.Contains(string(env), "BASEHARBOR_POSTGRES_PASSWORD=postgres\n") {
		t.Fatal("runtime environment uses an insecure default password")
	}
}

func TestEnsureFilesPreservesExistingSecret(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureFiles(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing runtime environment was overwritten")
	}
}

func TestEnsureFilesWithPortsWritesSelectedPorts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFilesWithPorts(dir, Ports{Postgres: 15432, OpenBao: 18200})
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	text := string(env)
	if !strings.Contains(text, "BASEHARBOR_POSTGRES_PORT=15432\n") {
		t.Fatal("selected PostgreSQL port was not persisted")
	}
	if !strings.Contains(text, "BASEHARBOR_OPENBAO_PORT=18200\n") {
		t.Fatal("selected OpenBao port was not persisted")
	}
}

func TestEnsureFilesWithPortsRejectsDuplicatePort(t *testing.T) {
	if _, err := EnsureFilesWithPorts(filepath.Join(t.TempDir(), "runtime"), Ports{Postgres: 15432, OpenBao: 15432}); err == nil {
		t.Fatal("expected duplicate host port rejection")
	}
}

func TestEmbeddedComposeUsesLoopbackBindings(t *testing.T) {
	text := string(composeYAML)
	if !strings.Contains(text, "127.0.0.1:${BASEHARBOR_POSTGRES_PORT}:5432") {
		t.Fatal("postgres is not bound to loopback")
	}
	if !strings.Contains(text, "127.0.0.1:${BASEHARBOR_OPENBAO_PORT}:8200") {
		t.Fatal("openbao is not bound to loopback")
	}
	if strings.Contains(text, "-dev") {
		t.Fatal("openbao must not run in dev mode")
	}
}

func TestEmbeddedComposeUsesWritableOpenBaoFileStoragePath(t *testing.T) {
	text := string(composeYAML)
	if !strings.Contains(text, `"path":"/openbao/file"`) {
		t.Fatal("openbao file storage must use the image-managed /openbao/file path")
	}
	if !strings.Contains(text, "openbao-data:/openbao/file") {
		t.Fatal("openbao persistent volume must mount at /openbao/file")
	}
	if strings.Contains(text, "/openbao/data") {
		t.Fatal("openbao runtime must not use the non-image-managed /openbao/data path")
	}
}
