package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAppProjectFindsComposeBackendsWorkloadAndSecretNames(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "compose.yaml"), `services:
  postgres:
    image: postgres:18-alpine
  valkey:
    image: valkey/valkey:8-alpine
  api:
    build: .
    ports:
      - "8080:8080"
  worker:
    image: example/worker:latest
`)
	mustWriteWizardTestFile(t, filepath.Join(dir, ".env.example"), `DATABASE_URL=
REDIS_URL=
OPENAI_API_KEY=
SMTP_PASSWORD=
PUBLIC_WEB_URL=http://localhost:8080
`)

	d, err := detectAppProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Postgres || !d.Redis {
		t.Fatalf("expected postgres and redis detection, got %+v", d)
	}
	if d.Compose != "compose.yaml" {
		t.Fatalf("expected compose.yaml, got %q", d.Compose)
	}
	if got := strings.Join(d.WorkloadServices, ","); got != "api,worker" {
		t.Fatalf("unexpected workload services %q", got)
	}
	if got := strings.Join(d.SecretCandidates, ","); got != "OPENAI_API_KEY,SMTP_PASSWORD" {
		t.Fatalf("unexpected secret candidates %q", got)
	}
}

func TestGuidedInitQuickUsesDetectionAndNeverCopiesSecretValues(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "docker-compose.yml"), `services:
  db:
    image: postgres:18-alpine
  cache:
    image: valkey/valkey:8-alpine
  web:
    build: .
`)
	mustWriteWizardTestFile(t, filepath.Join(dir, ".env.example"), `OPENAI_API_KEY=must-not-be-copied
SECRET_KEY=also-must-not-be-copied
`)

	withWizardTestDir(t, dir)
	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{
		"postgres:",
		"redis:",
		"secrets:",
		"- name: OPENAI_API_KEY",
		"- name: SECRET_KEY",
		"compose: docker-compose.yml",
		"- web",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "must-not-be-copied") {
		t.Fatalf("secret value leaked into manifest:\n%s", manifest)
	}
}

func TestGuidedInitInteractiveCanAcceptDetectedDefaults(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "compose.yaml"), `services:
  postgres:
    image: postgres:18-alpine
  app:
    build: .
`)
	mustWriteWizardTestFile(t, filepath.Join(dir, ".env.example"), "SMTP_PASSWORD=\n")
	withWizardTestDir(t, dir)

	oldInput := appInitInput
	appInitInput = strings.NewReader("\n\n\n\n\n\ny\n")
	t.Cleanup(func() { appInitInput = oldInput })

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "Analyzing repository") || !strings.Contains(text, "Manifest preview") {
		t.Fatalf("wizard output missing expected sections:\n%s", text)
	}
	data, err := os.ReadFile(filepath.Join(dir, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	if !strings.Contains(manifest, "- name: SMTP_PASSWORD") {
		t.Fatalf("expected detected secret in manifest:\n%s", manifest)
	}
}

func TestGuidedInitExplicitFlagsKeepDeterministicPath(t *testing.T) {
	dir := t.TempDir()
	withWizardTestDir(t, dir)
	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"demo", "--postgres", "--redis", "--require-secret", "API_TOKEN"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{"name: demo", "postgres:", "redis:", "- name: API_TOKEN"} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("deterministic manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestGuidedInitQuickFailsClosedOnAmbiguousCompose(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "compose.yaml"), "services:\n  web:\n    image: example/web\n")
	mustWriteWizardTestFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  api:\n    image: example/api\n")
	withWizardTestDir(t, dir)
	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "multiple Compose files") {
		t.Fatalf("expected ambiguous compose error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "baseharbor.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written on ambiguity, stat err=%v", statErr)
	}
}

func mustWriteWizardTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func withWizardTestDir(t *testing.T, dir string) {
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
			t.Errorf("restore cwd: %v", err)
		}
	})
}
