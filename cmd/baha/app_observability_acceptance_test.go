package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

func TestObservabilityFullStackAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_OBSERVABILITY_ACCEPTANCE") != "1" {
		t.Skip("full-stack observability acceptance requires BASEHARBOR_OBSERVABILITY_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect runtime: %v", err)
	}
	ensureRuntimeIntegrationTrustPlane(t, ctx)

	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.MetricsCollectSourcesEnv, "application,application-provider,platform-provider")
	t.Setenv(application.LogsEnabledEnv, "true")
	t.Setenv(application.LogsCollectSourcesEnv, "application,application-provider,platform-provider")
	t.Setenv(application.TracesEnabledEnv, "true")
	t.Setenv(application.TracesCollectSourcesEnv, "application,application-provider,platform-provider")

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	m := application.New("observability-fullstack-ci", "dev", true, true, false)
	m = application.WithWorkload(m, "compose.yaml", "api", "trace-probe")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithLogsCollection(m, "application")
	m = application.WithOTLPTelemetry(m, "traces")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(application.RepositoryManifestName, []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := telemetry.VerificationTracePayload(m)
	oldTraceID, err := hex.DecodeString(telemetry.ProbeTraceIDHex)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(root + "\x00" + t.Name()))
	traceIDBytes := append([]byte(nil), sum[:16]...)
	traceID := hex.EncodeToString(traceIDBytes)
	payload = bytes.Replace(payload, oldTraceID, traceIDBytes, 1)
	if !bytes.Contains(payload, traceIDBytes) {
		t.Fatal("failed to create unique workload verification trace payload")
	}
	if err := os.WriteFile("trace.bin", payload, 0o644); err != nil {
		t.Fatal(err)
	}

	composeYAML := `services:
  api:
    image: busybox:1.37
    command:
      - sh
      - -ec
      - |
        mkdir -p /www
        printf '# HELP baseharbor_acceptance_metric Full stack acceptance metric\n# TYPE baseharbor_acceptance_metric gauge\nbaseharbor_acceptance_metric 1\n' >/www/metrics
        echo baseharbor-observability-acceptance-api
        exec httpd -f -p 8080 -h /www
  trace-probe:
    image: curlimages/curl:8.16.0
    entrypoint: ["sh", "-c"]
    command: ["echo baseharbor-observability-acceptance-trace-probe; sleep 3600"]
    volumes:
      - ./trace.bin:/trace.bin:ro
`
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	defer func() {
		out.Reset()
		_ = runWithIO(context.Background(), []string{"app", "destroy", "--yes"}, &out, &out)
	}()

	if err := runWithIO(ctx, []string{"app", "apply"}, &out, &out); err != nil {
		t.Fatalf("full-stack observability apply failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"[VERIFIED] metrics",
		"1 source(s) scraped and ingested",
		"[VERIFIED] logs",
		"2 workload stream(s), 2 provider stream(s) ingested",
		"[VERIFIED] traces",
		"2 provider interaction trace(s) queryable",
		"[VERIFIED] telemetry",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("apply output missing %q:\n%s", want, out.String())
		}
	}

	assertSignalProviders(t, m, observability.SignalMetrics, []capability.ProviderKind{
		capability.ProviderOTelCollector,
		capability.ProviderLoki,
		capability.ProviderTempo,
	})
	assertSignalProviders(t, m, observability.SignalLogs, []capability.ProviderKind{
		capability.ProviderPostgreSQL,
		capability.ProviderValkey,
	})
	assertSignalProviders(t, m, observability.SignalTraces, []capability.ProviderKind{
		capability.ProviderPostgreSQL,
		capability.ProviderValkey,
	})

	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	stored, _, err := store.Load(m.Name)
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(store, stored)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, stored, files)
	if err != nil || !found {
		t.Fatalf("materialize workload: found=%v err=%v", found, err)
	}
	composeFiles := []string{workload.Compose, workload.Override}
	tracePost := `set -eu
test -n "$OTEL_EXPORTER_OTLP_ENDPOINT"
test -r "$OTEL_EXPORTER_OTLP_CERTIFICATE"
set -- --fail --silent --show-error --cacert "$OTEL_EXPORTER_OTLP_CERTIFICATE"
if [ -n "${OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE:-}" ]; then
  test -r "$OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE"
  test -r "$OTEL_EXPORTER_OTLP_CLIENT_KEY"
  set -- "$@" --cert "$OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE" --key "$OTEL_EXPORTER_OTLP_CLIENT_KEY"
fi
curl "$@" -H "Content-Type: application/x-protobuf" --data-binary @/trace.bin "${OTEL_EXPORTER_OTLP_ENDPOINT%/}/v1/traces"
`
	if _, err := compose.ExecProjectFiles(ctx, workload.Project, root, "trace-probe", composeFiles, "sh", "-ec", tracePost); err != nil {
		t.Fatalf("workload OTLP export through injected binding failed: %v", err)
	}
	if err := tracesprovider.VerifyTrace(ctx, stored, traceID); err != nil {
		t.Fatalf("workload OTLP trace was not queryable from Tempo: %v", err)
	}
}

func assertSignalProviders(t *testing.T, m application.Manifest, kind observability.SignalKind, want []capability.ProviderKind) {
	t.Helper()

	var placement capability.ProviderPlacement
	var err error
	switch kind {
	case observability.SignalMetrics:
		placement, err = application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	case observability.SignalLogs:
		placement, err = application.ResolveProviderPlacement(m, capability.ProviderLoki)
	case observability.SignalTraces:
		placement, err = application.ResolveProviderPlacement(m, capability.ProviderTempo)
	default:
		t.Fatalf("unsupported signal kind %q", kind)
	}
	if err != nil {
		t.Fatal(err)
	}

	sources, err := observability.List(kind, placement, []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[capability.ProviderKind]bool{}
	for _, source := range sources {
		got[source.Provider] = true
	}
	for _, provider := range want {
		if !got[provider] {
			t.Fatalf("%s registry missing provider %s: %#v", kind, provider, sources)
		}
	}
}
