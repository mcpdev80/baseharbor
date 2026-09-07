package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePostgresRuntimeIsolatedAndIdempotent(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}

	files, err := EnsurePostgresRuntime(store, m)
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
	if !strings.Contains(string(compose), "postgres-data:/var/lib/postgresql/data") {
		t.Fatal("dedicated postgres volume is missing")
	}

	firstEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firstEnv), "POSTGRES_PASSWORD=baseharbor") {
		t.Fatal("runtime must use a generated password")
	}
	if err := os.WriteFile(files.Env, firstEnv, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsurePostgresRuntime(store, m); err != nil {
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

func TestEnsurePostgresRuntimeRejectsUnsupportedDesiredServices(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, true, false)
	if _, err := EnsurePostgresRuntime(store, m); !errors.Is(err, ErrUnsupportedService) {
		t.Fatalf("expected ErrUnsupportedService, got %v", err)
	}
}

func TestRuntimeProjectNameIncludesApplicationAndEnvironment(t *testing.T) {
	m := New("mailflow", "prod", true, false, false)
	if got := RuntimeProjectName(m); got != "baseharbor-mailflow-prod" {
		t.Fatalf("unexpected project name %q", got)
	}
}
