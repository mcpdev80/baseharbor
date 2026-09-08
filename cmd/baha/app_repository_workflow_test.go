package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRepositoryManifestResolvesWithoutApplicationName(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	m := application.New("mailflow", "dev", true, false, false)
	if err := os.WriteFile("baseharbor.yaml", []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "frontend", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "show"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "name: mailflow") {
		t.Fatalf("repo manifest was not resolved: %s", out.String())
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "plan"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Plan for mailflow (dev)") {
		t.Fatalf("repo plan did not resolve application: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(nested, ".baseharbor")); !os.IsNotExist(err) {
		t.Fatalf("state must not be rooted in nested working directory: %v", err)
	}
}

func TestRepositoryEnvWithoutNameMasksNamedServiceURLs(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	m := application.New("mailflow", "dev", false, false, false)
	m = application.WithPostgresInstances(m, "primary", "analytics")
	if err := os.WriteFile("baseharbor.yaml", []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	store := application.DefaultStore()
	if _, err := store.Sync(m); err != nil {
		t.Fatal(err)
	}
	if _, err := application.EnsureRuntime(store, m); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "env"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, key := range []string{"DATABASE_PRIMARY_URL=<masked>", "DATABASE_ANALYTICS_URL=<masked>"} {
		if !strings.Contains(text, key) {
			t.Fatalf("named credential URL was not masked: %s", text)
		}
	}
	if strings.Contains(text, "postgresql://") {
		t.Fatalf("repository env leaked PostgreSQL credentials: %s", text)
	}
}

func TestAppInitCreatesCommitFriendlyRepositoryManifest(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "init", "demo", "--postgres", "--redis"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("baseharbor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("repository manifest permissions = %o want 644", info.Mode().Perm())
	}
	m, err := application.LoadManifestFile("baseharbor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "demo" || len(application.PostgresInstanceNames(m)) != 1 || len(application.RedisInstanceNames(m)) != 1 {
		t.Fatalf("unexpected generated manifest: %#v", m)
	}
	if err := runWithIO(context.Background(), []string{"app", "init", "demo"}, &out, &out); err == nil {
		t.Fatal("expected init to refuse overwriting repository manifest")
	}
}
