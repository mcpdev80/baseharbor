package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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


type kubernetesReferenceAcceptance struct {
	Version              int    `json:"version"`
	ReferenceApplication string `json:"reference_application"`
	Source struct {
		Compose             string   `json:"compose"`
		WorkloadServices    []string `json:"workload_services"`
		RepositoryOwned     bool     `json:"repository_owned"`
		MustRemainUnchanged bool     `json:"must_remain_unchanged"`
	} `json:"source"`
	KubernetesRealization struct {
		NamespaceIsExternalInput bool     `json:"namespace_is_external_input"`
		RequiredNamespacedKinds  []string `json:"required_namespaced_kinds"`
		ForbiddenClusterKinds    []string `json:"forbidden_cluster_scoped_kinds"`
		Workload struct {
			Service       string `json:"service"`
			ContainerPort int    `json:"container_port"`
			BuildContext  string `json:"build_context"`
		} `json:"workload"`
		Bindings        []string `json:"bindings"`
		OwnershipLabels []string `json:"ownership_labels"`
	} `json:"kubernetes_realization"`
}

// TestKubernetesReferenceDemoTargetAcceptance freezes the expected shape of
// the first Kubernetes realization without making that shape part of the
// application contract. The file is test acceptance data only; provider
// implementation may evolve internally as long as these semantics hold.
func TestKubernetesReferenceDemoTargetAcceptance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "kubernetes", "reference-demo-acceptance.json"))
	if err != nil {
		t.Fatal(err)
	}

	var spec kubernetesReferenceAcceptance
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Version != 1 {
		t.Fatalf("acceptance version = %d, want 1", spec.Version)
	}
	if spec.ReferenceApplication != "baseharbor-demo" {
		t.Fatalf("reference application = %q", spec.ReferenceApplication)
	}
	if spec.Source.Compose != "compose.yaml" ||
		!reflect.DeepEqual(spec.Source.WorkloadServices, []string{"demo-app"}) ||
		!spec.Source.RepositoryOwned ||
		!spec.Source.MustRemainUnchanged {
		t.Fatalf("source contract = %#v", spec.Source)
	}
	if !spec.KubernetesRealization.NamespaceIsExternalInput {
		t.Fatal("Kubernetes namespace must remain deployment/platform input")
	}

	requiredKinds := map[string]bool{
		"Deployment": false,
		"Service":    false,
		"ConfigMap":  false,
		"Secret":     false,
	}
	for _, kind := range spec.KubernetesRealization.RequiredNamespacedKinds {
		if _, ok := requiredKinds[kind]; ok {
			requiredKinds[kind] = true
		}
	}
	for kind, found := range requiredKinds {
		if !found {
			t.Fatalf("required namespaced kind %s missing from acceptance", kind)
		}
	}

	forbidden := map[string]bool{
		"Namespace":                false,
		"ClusterRole":              false,
		"ClusterRoleBinding":       false,
		"CustomResourceDefinition": false,
		"StorageClass":             false,
		"GatewayClass":             false,
	}
	for _, kind := range spec.KubernetesRealization.ForbiddenClusterKinds {
		if _, ok := forbidden[kind]; ok {
			forbidden[kind] = true
		}
	}
	for kind, found := range forbidden {
		if !found {
			t.Fatalf("cluster-scoped kind %s is not explicitly forbidden", kind)
		}
	}

	if spec.KubernetesRealization.Workload.Service != "demo-app" ||
		spec.KubernetesRealization.Workload.ContainerPort != 8080 ||
		spec.KubernetesRealization.Workload.BuildContext != "./demo-app" {
		t.Fatalf("workload target = %#v", spec.KubernetesRealization.Workload)
	}

	for _, requiredBinding := range []string{
		"DATABASE_URL",
		"REDIS_URL",
		"S3_ENDPOINT",
		"S3_BUCKET",
		"APP_SECRET",
	} {
		if !containsString(spec.KubernetesRealization.Bindings, requiredBinding) {
			t.Fatalf("required application binding %s missing", requiredBinding)
		}
	}
	for _, requiredLabel := range []string{
		"app.kubernetes.io/managed-by",
		"baseharbor.io/application",
		"baseharbor.io/environment",
	} {
		if !containsString(spec.KubernetesRealization.OwnershipLabels, requiredLabel) {
			t.Fatalf("required ownership label %s missing", requiredLabel)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
