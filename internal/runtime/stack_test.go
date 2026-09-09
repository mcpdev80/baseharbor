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

func TestDefaultStateDirUsesUserDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "baseharbor", "runtime")
	if dir != want {
		t.Fatalf("state dir = %q, want %q", dir, want)
	}
}

func TestDefaultStateDirUsesXDGDataHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, _ := os.Getwd()
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "baseharbor", "runtime")
	if dir != want {
		t.Fatalf("state dir = %q, want %q", dir, want)
	}
}

func TestStateDirOverrideWins(t *testing.T) {
	override := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("BASEHARBOR_STATE_DIR", override)
	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dir != override {
		t.Fatalf("state dir = %q, want override %q", dir, override)
	}
}

func TestLegacyStateIsReusedWhenGlobalStateIsAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, _ := os.Getwd()
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	legacy := filepath.Join(work, legacyStateDir)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, envName), []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dir != legacyStateDir {
		t.Fatalf("state dir = %q, want legacy %q", dir, legacyStateDir)
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
