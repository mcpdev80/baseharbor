package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestKubernetesAdoptionContractReferenceDemoKeepsRepositoryWorkloadUnchanged
// freezes the input-side portability contract before a Kubernetes runtime
// provider exists.
//
// The reference application remains an ordinary Compose repository. Runtime
// providers may translate its detected workload, but they must not require the
// application to rewrite compose.yaml or introduce Kubernetes-specific
// application intent.
func TestKubernetesAdoptionContractReferenceDemoKeepsRepositoryWorkloadUnchanged(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yaml")

	// Kept intentionally aligned with the root workload shape in
	// mcpdev80/baseharbor-demo. Infrastructure services are present in the
	// repository, but only demo-app is the application workload.
	mustWriteWizardTestFile(t, composePath, `services:
  demo-app:
    build:
      context: ./demo-app
    ports:
      - "${DEMO_HTTP_PORT:-8080}:8080"
    environment:
      PORT: "8080"
      DATABASE_URL: ${DATABASE_URL:-postgres://demo:demo@postgres:5432/demo?sslmode=disable}
      REDIS_URL: ${REDIS_URL:-redis://valkey:6379/0}
      S3_ENDPOINT: ${S3_ENDPOINT:-http://object-storage:9000}
      S3_ACCESS_KEY: ${S3_ACCESS_KEY:-demo}
      S3_SECRET_KEY: ${S3_SECRET_KEY:-demo-demo-demo}
      S3_BUCKET: ${S3_BUCKET:-uploads}
      APP_SECRET: ${APP_SECRET:-standalone-demo-secret}
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-}
      COMPANION_URL: ${COMPANION_URL:-}
    restart: unless-stopped

  postgres:
    image: postgres:17-alpine
    profiles: ["standalone"]
    environment:
      POSTGRES_USER: demo
      POSTGRES_PASSWORD: demo
      POSTGRES_DB: demo

  valkey:
    image: valkey/valkey:8-alpine
    profiles: ["standalone"]

  object-storage:
    image: minio/minio:RELEASE.2024-12-18T13-15-44Z
    profiles: ["standalone"]
    command: server /data
`)
	if err := os.MkdirAll(filepath.Join(root, "demo-app"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteWizardTestFile(t, filepath.Join(root, "demo-app", "Dockerfile"), "FROM scratch\n")

	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(before)

	detected, err := detectAppProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if detected.Compose != "compose.yaml" {
		t.Fatalf("selected Compose = %q, want compose.yaml", detected.Compose)
	}
	if !reflect.DeepEqual(detected.WorkloadServices, []string{"demo-app"}) {
		t.Fatalf("workload services = %#v, want [demo-app]", detected.WorkloadServices)
	}
	if !reflect.DeepEqual(
		detected.InfrastructureServices,
		[]string{"object-storage", "postgres", "valkey"},
	) {
		t.Fatalf("infrastructure services = %#v", detected.InfrastructureServices)
	}
	if !detected.Postgres || !detected.Redis || !detected.ObjectStorage {
		t.Fatalf("reference capabilities were not detected: %+v", detected)
	}

	withWizardTestDir(t, root)
	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(
		context.Background(),
		[]string{"--quick"},
		&out,
		&bytes.Buffer{},
	); err != nil {
		t.Fatalf("quick adoption failed: %v\n%s", err, out.String())
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := sha256.Sum256(after)
	if beforeHash != afterHash {
		t.Fatal("repository-owned compose.yaml changed during adoption")
	}

	m, err := application.LoadManifestFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if m.Workload.Compose != "compose.yaml" {
		t.Fatalf("manifest workload compose = %q", m.Workload.Compose)
	}
	if !reflect.DeepEqual(m.Workload.Services, []string{"demo-app"}) {
		t.Fatalf("manifest workload services = %#v", m.Workload.Services)
	}
	if !m.Services.Postgres || !m.Services.Redis || !m.Services.ObjectStorage {
		t.Fatalf("portable service intent missing: %#v", m.Services)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	manifestText := strings.ToLower(string(manifestBytes))
	for _, forbidden := range []string{
		"kubernetes",
		"deployment:",
		"statefulset:",
		"namespace:",
		"helm:",
	} {
		if strings.Contains(manifestText, forbidden) {
			t.Fatalf("runtime-specific Kubernetes intent leaked into baseharbor.yaml via %q:\n%s", forbidden, string(manifestBytes))
		}
	}
}
