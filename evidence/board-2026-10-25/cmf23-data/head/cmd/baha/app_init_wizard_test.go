package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
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
	if !d.SQL || !d.Cache {
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
		"sql:",
		"cache:",
		"components:",
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
	appInitInput = strings.NewReader(strings.Repeat("\n", 32))
	t.Cleanup(func() { appInitInput = oldInput })

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "Analyzing repository") || !strings.Contains(text, "Adoption summary") {
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

func TestGuidedInitDeterministicPathRequiresExplicitContract(t *testing.T) {
	dir := t.TempDir()
	withWizardTestDir(t, dir)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"demo", "-e", "dev"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires an explicit capability or workload selection") {
		t.Fatalf("expected explicit-contract failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written without explicit deterministic intent, stat err=%v", statErr)
	}
}

func TestGuidedInitDeterministicCapabilitiesFailClosedWhenRepositoryWorkloadExists(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "compose.yaml"), "services:\n  app:\n    image: example/app\n")
	withWizardTestDir(t, dir)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"demo", "--sql", "--cache"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "repository workload evidence") {
		t.Fatalf("expected unresolved workload failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written with unresolved repository workload, stat err=%v", statErr)
	}
}

func TestGuidedInitDeterministicExplicitWorkloadSelectionSucceeds(t *testing.T) {
	dir := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(dir, "compose.yaml"), "services:\n  app:\n    image: example/app\n")
	withWizardTestDir(t, dir)

	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"demo", "--sql", "--workload-source", "compose:compose.yaml", "--workload-component", "app"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m, err := application.LoadManifestFile(filepath.Join(dir, application.RepositoryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	components := application.WorkloadComponentNames(m)
	if len(components) != 1 || components[0] != "app" {
		t.Fatalf("unexpected explicit workload: %#v", m.Workload)
	}
	if strings.Contains(m.YAML(), "compose:") {
		t.Fatalf("source identity leaked into portable manifest:\n%s", m.YAML())
	}
}

func TestGuidedInitExplicitFlagsKeepDeterministicPath(t *testing.T) {
	dir := t.TempDir()
	withWizardTestDir(t, dir)
	var out bytes.Buffer
	if err := appGuidedInitCommand().Run(context.Background(), []string{"demo", "--sql", "--cache", "--require-secret", "API_TOKEN"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, want := range []string{"name: demo", "sql:", "cache:", "- name: API_TOKEN"} {
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
	if m.Services.SQL || m.Services.Cache || m.Services.Secrets {
		t.Fatalf("quick init invented backend capability: %#v", m.Services)
	}
	components := application.WorkloadComponentNames(m)
	if len(components) != 1 || components[0] != "api" {
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

func TestQuickInitFailsClosedOnAmbiguousComposeServiceRole(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
  database:
    image: company/custom-database
`)
	withWizardTestDir(t, root)

	var out bytes.Buffer
	err := appGuidedInitCommand().Run(context.Background(), []string{"--quick"}, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ambiguous Compose service classification") {
		t.Fatalf("expected ambiguous service classification failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, application.RepositoryManifestName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest should not be written on ambiguous classification, stat err=%v", statErr)
	}
}

func TestPromptCapabilityListNonTTYKeepsDetectedDefaults(t *testing.T) {
	oldInput := appInitInput
	defer func() { appInitInput = oldInput }()

	input := strings.NewReader("\n")
	appInitInput = input
	reader := bufio.NewReader(input)
	defaults := make([]bool, guidedCapabilityCount)
	defaults[guidedCapabilitySQL] = true
	defaults[guidedCapabilityCache] = true
	defaults[guidedCapabilitySecrets] = true

	got, err := promptCapabilityList(reader, io.Discard, defaults, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, defaults) {
		t.Fatalf("selection = %#v, want %#v", got, defaults)
	}
}

func TestPromptCapabilityListNonTTYAcceptsExplicitSelection(t *testing.T) {
	oldInput := appInitInput
	defer func() { appInitInput = oldInput }()

	input := strings.NewReader("1,3,4,5,6,7,12\n")
	appInitInput = input
	reader := bufio.NewReader(input)

	got, err := promptCapabilityList(reader, io.Discard, make([]bool, guidedCapabilityCount), true)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]bool, guidedCapabilityCount)
	for _, index := range []int{
		guidedCapabilitySQL,
		guidedCapabilityDurableKeyValue,
		guidedCapabilityDocumentDatabase,
		guidedCapabilityMessagingQueue,
		guidedCapabilityMessagingPubSub,
		guidedCapabilityMessagingStream,
		guidedCapabilityOTLP,
	} {
		want[index] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selection = %#v, want %#v", got, want)
	}
}

func TestPromptAmbiguousComposeServicesConfirmsWorkloadSelection(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("2\n"))
	var out bytes.Buffer
	got, err := promptAmbiguousComposeServices(reader, &out, []string{"database", "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "worker" {
		t.Fatalf("selected workload services = %#v", got)
	}
}

func TestPromptSecretPoliciesSupportsRenameOptionalGenerateAndSkip(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(
		"y\nRENAMED_TOKEN\nn\n2\n" +
			"n\n",
	))
	var out bytes.Buffer
	policies, err := promptSecretPolicies(
		reader,
		&out,
		[]string{"API_TOKEN", "SMTP_PASSWORD"},
		map[string]string{
			"API_TOKEN":     ".env.example variable API_TOKEN",
			"SMTP_PASSWORD": ".env.example variable SMTP_PASSWORD",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("policies = %#v", policies)
	}
	policy := policies[0]
	if policy.Name != "RENAMED_TOKEN" || policy.Required || policy.Provision != "generate" {
		t.Fatalf("policy = %#v", policy)
	}
	m := detectedApplicationManifest("demo", "dev", false, false, false, true, true)
	m = applyGuidedSecretPolicies(m, policies)
	if len(m.Secrets.Required) != 0 || len(m.Secrets.Optional) != 1 {
		t.Fatalf("secret contract = %#v", m.Secrets)
	}
	if m.Secrets.Optional[0].Name != "RENAMED_TOKEN" || m.Secrets.Optional[0].Generate == nil {
		t.Fatalf("optional generated secret = %#v", m.Secrets.Optional[0])
	}
	if strings.Contains(out.String(), "must-not-be-visible") {
		t.Fatal("secret value leaked")
	}
}

func TestGuidedSecretSummaryShowsPolicyWithoutValues(t *testing.T) {
	var out bytes.Buffer
	printGuidedSecretSummary(&out, []guidedSecretPolicy{
		{Name: "API_TOKEN", Required: true, Provision: "prompt"},
		{Name: "SESSION_SECRET", Required: true, Provision: "generate"},
		{Name: "OPTIONAL_TOKEN", Required: false, Provision: "later"},
	})
	text := out.String()
	for _, want := range []string{
		"API_TOKEN: required for startup; ask securely during first apply",
		"SESSION_SECRET: required for startup; generate automatically",
		"OPTIONAL_TOKEN: optional; configure later",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q:\n%s", want, text)
		}
	}
}

func TestAdoptionSummaryMinimal(t *testing.T) {
	m := detectedApplicationManifest("demo", "dev", false, false, false, false, true)
	m = application.WithWorkloadComponents(m, "api")
	var out bytes.Buffer
	printAdoptionSummary(&out, m, appProjectDetection{}, nil)
	text := out.String()
	for _, want := range []string{
		"Adoption summary",
		"Application",
		"Name          demo",
		"Environment   dev",
		"Workload",
		"Components    api",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "version: 1") {
		t.Fatalf("normal summary unexpectedly dumped YAML:\n%s", text)
	}
}

func TestAdoptionSummaryFullyPopulated(t *testing.T) {
	m := detectedApplicationManifest("demo", "dev", true, true, true, true, true)
	m.Services.SQLManagementUI = true
	m.Services.CacheManagementUI = true
	m.Services.ObjectStorageManagementUI = true
	m.Services.SecretsManagementUI = true
	m.Services.IdentityManagementUI = true
	m.Services.ObservabilityManagementUI = true
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithOTLPTelemetry(m, "traces")
	m = application.WithLogsCollection(m, "application")
	m = application.WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create")
	policies := []guidedSecretPolicy{{Name: "APP_SECRET", Required: true, Provision: "prompt"}}

	var out bytes.Buffer
	printAdoptionSummary(&out, m, appProjectDetection{
		SQL: true, Cache: true, ObjectStorage: true,
	}, policies)
	text := out.String()
	for _, want := range []string{
		"Managed services",
		"SQL Database        detected and confirmed",
		"Cache               detected and confirmed",
		"Object Storage      detected and confirmed",
		"Management UIs",
		"PostgreSQL    pgAdmin",
		"Cache         Redis Commander",
		"Object Storage provider administration UI",
		"Secrets       OpenBao UI",
		"Identity      provider administration UI",
		"Observability Prometheus UI",
		"Observability",
		"Metrics       expose OpenMetrics HTTP (recommended /metrics)",
		"BaseHarbor collects workload + supported managed-provider metrics",
		"Logs          write application logs to stdout/stderr",
		"BaseHarbor collects workload + supported managed-provider logs",
		"Traces/OTLP   traces",
		"BaseHarbor injects the OTLP endpoint/trust binding",
		"no Prometheus, Loki/Alloy, Tempo or Collector configuration in application code",
		"APP_SECRET: required for startup; ask securely during first apply",
		"Runtime permissions",
		"object-storage.s3/v1",
		"operations: runtime.create",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q:\n%s", want, text)
		}
	}
}

func TestBuildGuidedInitManifestIncludesAllV0419ServiceFamilies(t *testing.T) {
	selection := guidedInitSelection{
		name:                         "demo",
		environment:                  "dev",
		selected:                     make([]bool, guidedCapabilityCount),
		keyValueInstances:            []string{"durable"},
		documentDatabaseInstances:    []string{"documents"},
		messagingQueueInstances:      []string{"jobs"},
		messagingPubSubInstances:     []string{"events"},
		messagingStreamInstances:     []string{"audit"},
		keyValueManagementUI:         true,
		documentDatabaseManagementUI: true,
		messagingManagementUI:        true,
	}
	for _, index := range []int{
		guidedCapabilityDurableKeyValue,
		guidedCapabilityDocumentDatabase,
		guidedCapabilityMessagingQueue,
		guidedCapabilityMessagingPubSub,
		guidedCapabilityMessagingStream,
	} {
		selection.selected[index] = true
	}

	m, err := buildGuidedInitManifest(bufio.NewReader(strings.NewReader("")), io.Discard, appProjectDetection{}, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := application.KeyValueInstanceNames(m); !reflect.DeepEqual(got, []string{"durable"}) {
		t.Fatalf("durable key-value instances = %#v", got)
	}
	if got := application.DocumentDatabaseInstanceNames(m); !reflect.DeepEqual(got, []string{"documents"}) {
		t.Fatalf("document database instances = %#v", got)
	}
	if got := application.MessagingQueueInstanceNames(m); !reflect.DeepEqual(got, []string{"jobs"}) {
		t.Fatalf("messaging queue instances = %#v", got)
	}
	if got := application.MessagingPubSubInstanceNames(m); !reflect.DeepEqual(got, []string{"events"}) {
		t.Fatalf("messaging pubsub instances = %#v", got)
	}
	if got := application.MessagingStreamInstanceNames(m); !reflect.DeepEqual(got, []string{"audit"}) {
		t.Fatalf("messaging stream instances = %#v", got)
	}
	if !m.Services.KeyValueManagementUI || !m.Services.DocumentDatabaseManagementUI || !m.Services.MessagingManagementUI {
		t.Fatalf("management UI selection missing: %#v", m.Services)
	}
	yaml := m.YAML()
	for _, want := range []string{
		"key_value:",
		"document_database:",
		"messaging_queue:",
		"messaging_pubsub:",
		"messaging_stream:",
		"management_ui: true",
	} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("guided manifest missing %q:\n%s", want, yaml)
		}
	}
}

func TestBuildGuidedInitManifestPreservesExplicitHTTPSExposureEvidence(t *testing.T) {
	selection := guidedInitSelection{
		name:              "demo",
		environment:       "dev",
		compose:           "compose.yaml",
		workloadServices:  []string{"demo-app"},
		workloadProtocols: map[string]string{"demo-app": "https"},
		workloadPorts: []repositoryinspect.PortEvidence{
			{Path: "compose.yaml", Service: "demo-app", Value: "${DEMO_HTTPS_PORT:-8080}:8080"},
		},
		selected: make([]bool, guidedCapabilityCount),
	}
	m, err := buildGuidedInitManifest(bufio.NewReader(strings.NewReader("")), io.Discard, appProjectDetection{}, selection)
	if err != nil {
		t.Fatal(err)
	}
	want := []application.HTTPExposureRequirement{{
		Name: "demo-app", Service: "demo-app", Port: 8080, Protocol: "https", Visibility: "public",
	}}
	if !reflect.DeepEqual(m.Exposures, want) {
		t.Fatalf("exposures = %#v, want %#v", m.Exposures, want)
	}
}
