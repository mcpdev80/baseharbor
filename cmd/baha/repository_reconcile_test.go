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

func TestReportRepositoryContractEvolutionShowsNewCapability(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(manifestPath, []byte(`version: 1
app:
  name: demo
  environment: dev
services:
  sql:
    enabled: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("REDIS_URL=\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	reportRepositoryContractEvolution(context.Background(), &out, &errOut, resolvedApplication{
		ManifestPath:   manifestPath,
		FromRepository: true,
	})
	if errOut.Len() != 0 {
		t.Fatalf("unexpected warning: %s", errOut.String())
	}
	for _, want := range []string{"Application contract review:", "[NEW] cache.key-value consume", "no contract changes or runtime permissions"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestReportRepositoryContractEvolutionShowsRuntimeOperationWithoutGrantingIt(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(manifestPath, []byte(`version: 1
app:
  name: demo
  environment: dev
services:
  object_storage:
    enabled: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "storage.go"), []byte("package storage\nfunc create(c *S3) { c.CreateBucket(\"tenant\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	reportRepositoryContractEvolution(context.Background(), &out, &bytes.Buffer{}, resolvedApplication{
		ManifestPath:   manifestPath,
		FromRepository: true,
	})
	if !strings.Contains(out.String(), "runtime.create") {
		t.Fatalf("runtime operation hint missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "no contract changes or runtime permissions were applied") {
		t.Fatalf("authorization boundary missing:\n%s", out.String())
	}
}
