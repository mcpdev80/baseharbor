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
	if strings.Contains(string(compose), "ports:") {
		t.Fatal("application postgres must not publish a host port by default")
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
	if _, err := EnsureRuntime(store, m); err != nil {
		t.Fatal(err)
	}
	secondEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstEnv) != string(secondEnv) {
		t.Fatal("repeated convergence rotated credentials unexpectedly")
	}

	for _, path := range []string{files.Compose, files.Env} {
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
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("runtime compose missing %q", wanted)
		}
	}
	if strings.Contains(text, "ports:") {
		t.Fatal("application services must not publish host ports by default")
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "VALKEY_PASSWORD=") {
		t.Fatal("Valkey credential was not generated")
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

func TestEnsureRuntimeRejectsSecretsUntilImplemented(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, true)
	if _, err := EnsureRuntime(store, m); err == nil {
		t.Fatal("expected managed secrets to fail closed")
	}
}

func TestRuntimeProjectNameIncludesApplicationAndEnvironment(t *testing.T) {
	m := New("mailflow", "prod", true, false, false)
	if got := RuntimeProjectName(m); got != "baseharbor-mailflow-prod" {
		t.Fatalf("unexpected project name %q", got)
	}
}
