package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRuntimePostgresIsolatedAndIdempotent(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}

	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), `127.0.0.1:${POSTGRES_HOST_PORT}:5432`) {
		t.Fatal("application postgres must be published only on loopback")
	}
	if !strings.Contains(string(compose), "postgres-data:/var/lib/postgresql") {
		t.Fatal("dedicated postgres volume is missing or uses the pre-18 mount path")
	}
	if strings.Contains(string(compose), "postgres-data:/var/lib/postgresql/data") {
		t.Fatal("PostgreSQL 18 runtime must not use the pre-18 data volume mount")
	}

	firstEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firstEnv), "POSTGRES_PASSWORD=baseharbor") {
		t.Fatal("runtime must use a generated password")
	}
	if !strings.Contains(string(firstEnv), "POSTGRES_HOST_PORT=") {
		t.Fatal("runtime must allocate a local PostgreSQL port")
	}
	if _, err := EnsureRuntime(store, m); err != nil {
		t.Fatal(err)
	}
	secondEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstEnv) != string(secondEnv) {
		t.Fatal("repeated convergence rotated credentials or ports unexpectedly")
	}

	for _, path := range []string{files.Compose, files.Env, files.ApplicationEnv, files.Bindings} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s is accessible by group or others: %o", path, info.Mode().Perm())
		}
	}
}

func TestEnsureRuntimePostgresAndValkey(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, true, false)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, wanted := range []string{
		"postgres:18-alpine",
		"valkey/valkey:9.1.2-alpine",
		"postgres-data:/var/lib/postgresql",
		"valkey-data:/data",
		"appendonly yes",
		`127.0.0.1:${POSTGRES_HOST_PORT}:5432`,
		`127.0.0.1:${VALKEY_HOST_PORT}:6379`,
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("runtime compose missing %q", wanted)
		}
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"VALKEY_PASSWORD=", "POSTGRES_HOST_PORT=", "VALKEY_HOST_PORT="} {
		if !strings.Contains(string(env), wanted) {
			t.Fatalf("runtime env missing %q", wanted)
		}
	}
}

func TestEnsureRuntimeBackfillsPortsWithoutRotatingCredentials(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("legacy", "dev", true, true, false)
	dir := filepath.Join(store.Root, m.Name, "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := "POSTGRES_DB=legacy_dev\nPOSTGRES_USER=baseharbor\nPOSTGRES_PASSWORD=keep-postgres\nVALKEY_PASSWORD=keep-valkey\n"
	if err := os.WriteFile(filepath.Join(dir, "runtime.env"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, wanted := range []string{"POSTGRES_PASSWORD=keep-postgres", "VALKEY_PASSWORD=keep-valkey", "POSTGRES_HOST_PORT=", "VALKEY_HOST_PORT="} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("upgraded runtime env missing %q", wanted)
		}
	}
}

func TestEnsureRuntimeCreatesNativeApplicationContract(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, true, false)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	appEnv, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	text := string(appEnv)
	for _, wanted := range []string{
		"BASEHARBOR_APP_NAME=demo",
		"BASEHARBOR_ENVIRONMENT=dev",
		"BASEHARBOR_BINDINGS=",
		"DATABASE_URL=postgresql://",
		"REDIS_URL=redis://",
		"VALKEY_URL=redis://",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("application contract missing %q", wanted)
		}
	}
	for _, path := range []string{
		filepath.Join(files.Bindings, "postgres", "host"),
		filepath.Join(files.Bindings, "postgres", "port"),
		filepath.Join(files.Bindings, "postgres", "database"),
		filepath.Join(files.Bindings, "postgres", "username"),
		filepath.Join(files.Bindings, "postgres", "password"),
		filepath.Join(files.Bindings, "postgres", "uri"),
		filepath.Join(files.Bindings, "valkey", "host"),
		filepath.Join(files.Bindings, "valkey", "port"),
		filepath.Join(files.Bindings, "valkey", "password"),
		filepath.Join(files.Bindings, "valkey", "uri"),
		filepath.Join(files.Bindings, "metadata.json"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("binding %s: %v", path, err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("binding %s is accessible by group or others: %o", path, info.Mode().Perm())
		}
	}
}

func TestEnsureRuntimeValkeyOnly(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("cache", "dev", false, true, false)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compose), "postgres:") {
		t.Fatal("Valkey-only runtime unexpectedly contains PostgreSQL")
	}
	if !strings.Contains(string(compose), "valkey/valkey:9.1.2-alpine") {
		t.Fatal("Valkey-only runtime is missing Valkey")
	}
}

func TestEnsureRuntimeAllowsSecretsAlongsideMaterializedService(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, true)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "postgres:18-alpine") {
		t.Fatal("managed secrets changed the PostgreSQL runtime definition")
	}
}

func TestEnsureRuntimeRejectsSecretsOnlyUntilStandaloneLifecycleExists(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("secret-only", "dev", false, false, true)
	if _, err := EnsureRuntime(store, m); err == nil {
		t.Fatal("expected secrets-only runtime to fail closed")
	}
}

func TestRuntimeProjectNameIncludesApplicationAndEnvironment(t *testing.T) {
	m := New("mailflow", "prod", true, false, false)
	if got := RuntimeProjectName(m); got != "baseharbor-mailflow-prod" {
		t.Fatalf("unexpected project name %q", got)
	}
}
