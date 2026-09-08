package main

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

func TestRepositoryWorkflowRealLifecycle(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real repository lifecycle runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := bhruntime.DetectCompose(ctx); err != nil {
		t.Skipf("compose runtime unavailable: %v", err)
	}

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
	if err := runWithIO(ctx, []string{"app", "init", "repo-e2e", "--postgres", "--redis"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("repository apply failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Application repo-e2e is ready") {
		t.Fatalf("unexpected apply output: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".baseharbor", ".gitignore")); err != nil {
		t.Fatalf("generated state is not protected from Git: %v", err)
	}

	nested := filepath.Join(root, "frontend", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	for _, command := range [][]string{{"app", "status"}, {"app", "doctor"}} {
		out.Reset()
		if err := runWithIO(ctx, command, &out, &out); err != nil {
			t.Fatalf("%v failed: %v\n%s", command, err, out.String())
		}
	}
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "env", "--path"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	envPath := strings.TrimSpace(out.String())
	if !strings.HasPrefix(envPath, filepath.Join(root, ".baseharbor")) {
		t.Fatalf("environment path is not anchored to repository root: %s", envPath)
	}
	if _, err := os.Stat(filepath.Join(nested, ".baseharbor")); !os.IsNotExist(err) {
		t.Fatalf("nested command created nested BaseHarbor state: %v", err)
	}

	out.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("repository destroy failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
		t.Fatalf("repository manifest was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".baseharbor", "apps", "repo-e2e")); !os.IsNotExist(err) {
		t.Fatalf("managed application state remains after destroy: %v", err)
	}
}
