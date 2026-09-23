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

func TestGuidedInitQuickDoesNotPromoteHeuristicSecrets(t *testing.T) {
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
	for _, forbidden := range []string{"- name: OPENAI_API_KEY", "- name: SECRET_KEY", "secrets:\n    enabled: true"} {
		if strings.Contains(manifest, forbidden) {
			t.Fatalf("quick init promoted heuristic secret evidence %q:\n%s", forbidden, manifest)
		}
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

func TestQuickInitPreservesWorkloadOnlyRepository(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, err := application.LoadManifestFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if m.Services.Postgres || m.Services.Redis || m.Services.Secrets {
		t.Fatalf("quick init invented backend capability: %#v", m.Services)
	}
	if len(m.Workload.Services) != 1 || m.Workload.Services[0] != "api" {
		t.Fatalf("workload = %#v", m.Workload)
	}
}

func TestQuickInitDoesNotPromoteSuggestedCapability(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"pg":"latest"}}`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no unambiguous application requirements were detected") {
		t.Fatalf("expected fail-closed quick init, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written from suggested evidence, stat err=%v", statErr)
	}
}


func TestQuickInitIsolatesMixedComposeInfrastructureAndGeneratesDetectedIntent(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  demo-app:
    build: .
    ports:
      - "8080:8080"
  postgres:
    image: postgres:18
  valkey:
    image: valkey/valkey:8
  object-storage:
    image: quay.io/minio/minio:latest
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "main.go"), `package main
func metrics() string { return "/metrics" }
`)
	mustWriteWizardTestFile(t, filepath.Join(root, ".env.example"), "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=\n")
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{
		"- demo-app",
		"object_storage:",
		"metrics:",
		"path: /metrics",
		"telemetry:",
		"- traces",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, forbidden := range []string{"- postgres", "- valkey", "- object-storage"} {
		if strings.Contains(manifest, forbidden) {
			t.Fatalf("repository infrastructure leaked into workload %q:\n%s", forbidden, manifest)
		}
	}
}

func TestQuickInitFailsClosedOnAmbiguousMetricsTarget(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  worker:
    image: example/worker
    ports:
      - "9090:9090"
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "main.go"), `package main
const metricsPath = "/metrics"
`)
	withWizardTestDir(t, root)

	err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "metrics endpoint was detected") {
		t.Fatalf("expected fail-closed metrics ambiguity, got %v", err)
	}
}

func TestQuickInitScopesConcreteRuntimePermissionToSingleWorkload(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "storage.go"), `package main
func provision(client *S3Client) { client.CreateBucket("tenant") }
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{"runtime:", "object-storage.s3/v1", "runtime.create", "- api"} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("runtime intent missing %q:\n%s", want, manifest)
		}
	}
}


func TestQuickInitIsolatesMixedComposeInfrastructure(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    build: .
    ports:
      - "8080:8080"
  postgres:
    image: postgres:18
  cache:
    image: valkey/valkey:8
  object-storage:
    image: minio/minio
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, err := application.LoadManifestFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Workload.Services, ","); got != "api" {
		t.Fatalf("workload services = %q, want api", got)
	}
	if !m.Services.Postgres || !m.Services.Redis || !m.Services.ObjectStorage {
		t.Fatalf("expected managed SQL/cache/object-storage intent: %#v", m.Services)
	}
}

func TestQuickInitGeneratesDetectedMetricsAndOTLPSignal(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
`)
	mustWriteWizardTestFile(t, filepath.Join(root, ".env.example"), "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=\n")
	mustWriteWizardTestFile(t, filepath.Join(root, "main.go"), `package main
func routes() {
	router.GET("/metrics", metricsHandler)
}
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, err := application.LoadManifestFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Metrics.Sources) != 1 || m.Metrics.Sources[0].Service != "api" || m.Metrics.Sources[0].Port != 8080 || m.Metrics.Sources[0].Path != "/metrics" {
		t.Fatalf("metrics = %#v", m.Metrics)
	}
	if m.Telemetry.OTLP == nil || strings.Join(m.Telemetry.OTLP.Signals, ",") != "traces" {
		t.Fatalf("telemetry = %#v", m.Telemetry)
	}
}

func TestQuickInitGeneratesRuntimePermissionOnlyFromConcreteOperationEvidence(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "storage.go"), `package app
func ensureBucket(client *S3Client) {
	client.CreateBucket("tenant")
}
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, err := application.LoadManifestFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Runtime.Permissions) != 1 {
		t.Fatalf("runtime permissions = %#v", m.Runtime.Permissions)
	}
	p := m.Runtime.Permissions[0]
	if p.Capability != "object-storage.s3/v1" || strings.Join(p.Services, ",") != "api" || strings.Join(p.Operations, ",") != "runtime.create" {
		t.Fatalf("runtime permission = %#v", p)
	}
}

func TestQuickInitFailsClosedOnAmbiguousDetectedMetricsTarget(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  admin:
    image: example/admin
    ports:
      - "9090:9090"
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "main.go"), `package main
func routes() { router.GET("/metrics", metricsHandler) }
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "metrics endpoint was detected") {
		t.Fatalf("expected ambiguous metrics failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written on ambiguity, stat err=%v", statErr)
	}
}
