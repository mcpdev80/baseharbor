package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeRepositoryManifestForUpQuickCreatesDetectedContract(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(`services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  postgres:
    image: postgres:18
  redis:
    image: valkey/valkey:8
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("SECRET_KEY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	initialized, err := initializeRepositoryManifestForUp(context.Background(), strings.NewReader(""), &out, &out, runtimeUpOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !initialized {
		t.Fatal("expected application project to be initialized")
	}
	data, err := os.ReadFile(filepath.Join(root, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"postgres:", "redis:", "SECRET_KEY", "compose.yaml", "api"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated manifest missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(out.String(), "Creating the application contract from detected safe defaults") {
		t.Fatalf("output missing quick-init explanation:\n%s", out.String())
	}
}

func TestInitializeRepositoryManifestForUpIgnoresDirectoryWithoutProjectSignals(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	initialized, err := initializeRepositoryManifestForUp(context.Background(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runtimeUpOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if initialized {
		t.Fatal("unexpected application detection in empty directory")
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); !os.IsNotExist(err) {
		t.Fatalf("baseharbor.yaml unexpectedly materialized: %v", err)
	}
}

func TestInitializeRepositoryManifestForUpQuickFailsClosedOnAmbiguousCompose(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	for _, name := range []string{"compose.yaml", "docker-compose.yml"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("services:\n  api:\n    image: example/api\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	initialized, err := initializeRepositoryManifestForUp(context.Background(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runtimeUpOptions{Yes: true})
	if !initialized {
		t.Fatal("expected ambiguous application project to be detected")
	}
	if err == nil || !strings.Contains(err.Error(), "multiple Compose files") {
		t.Fatalf("error = %v, want ambiguous Compose failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "baseharbor.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("baseharbor.yaml unexpectedly materialized: %v", statErr)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
