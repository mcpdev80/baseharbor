package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func TestAppInspectCommandIsReadOnlyAndReportsConfidence(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectTestFile(t, root, "compose.yaml", `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  db:
    image: postgres:17
`)
	mustWriteInspectTestFile(t, root, ".env.example", "DATABASE_URL=\nOPENAI_API_KEY=\n")

	var out bytes.Buffer
	if err := appInspectCommand().Run(context.Background(), []string{root}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Capabilities:", "SQL Database", "OPENAI_API_KEY", "No changes were made."} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); !os.IsNotExist(err) {
		t.Fatalf("inspect mutated repository, stat err = %v", err)
	}
}

func TestAppInspectJSONUsesSharedResult(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectTestFile(t, root, "package.json", `{"dependencies":{"ioredis":"latest"}}`)

	var out bytes.Buffer
	if err := appInspectCommand().Run(context.Background(), []string{"--json", root}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var result repositoryinspect.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Capability != "cache.key-value" ||
		result.Findings[0].Confidence != repositoryinspect.ConfidenceSuggested {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppInspectRejectsUnknownOptions(t *testing.T) {
	err := appInspectCommand().Run(context.Background(), []string{"--write"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error")
	}
}

func mustWriteInspectTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}


func TestAppInspectVerboseKeepsDetailedEvidence(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectTestFile(t, root, "compose.yaml", `services:
  api:
    image: example/api
  db:
    image: postgres:18
`)

	var concise bytes.Buffer
	if err := appInspectCommand().Run(context.Background(), []string{root}, &concise, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(concise.String(), "compose service db matches database.sql") {
		t.Fatalf("concise output leaked detailed evidence:\n%s", concise.String())
	}

	var verbose bytes.Buffer
	if err := appInspectCommand().Run(context.Background(), []string{"--verbose", root}, &verbose, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(verbose.String(), "compose service db matches database.sql") {
		t.Fatalf("verbose output missing detailed evidence:\n%s", verbose.String())
	}
}

func TestAppInspectSummarizesAmbiguousComposeRoles(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectTestFile(t, root, "compose.yaml", `services:
  api:
    image: example/api
  db:
    image: postgres:18
`)
	mustWriteInspectTestFile(t, root, "deploy/docker-compose.yml", `services:
  worker:
    image: example/worker
  object-storage:
    image: minio/minio
`)

	var out bytes.Buffer
	if err := appInspectCommand().Run(context.Background(), []string{root}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"2 candidates", "workload: api", "infrastructure: db", "workload: worker", "infrastructure: object-storage"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
