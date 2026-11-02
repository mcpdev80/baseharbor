package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

func TestObservabilityFullStackAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_OBSERVABILITY_ACCEPTANCE") != "1" {
		t.Skip("full-stack observability acceptance requires BASEHARBOR_OBSERVABILITY_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("detect runtime: %v", err)
	}
	ensureRuntimeIntegrationTrustPlane(t, ctx)

	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
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
	m = application.WithWorkloadComponents(m, "api", "trace-probe")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithLogsCollection(m, "application")
	m = application.WithOTLPTelemetry(m, "traces")
	m = application.WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, application.RepositoryManifestName, m, 0o644); err != nil {
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
	composeYAML := `services:
  api:
    image: docker.io/library/python:3.13-alpine
    command:
      - python
      - -u
      - -c
      - |
        import os
        import ssl
        import threading
        import time
        from http.server import BaseHTTPRequestHandler, HTTPServer

        body = b"baseharbor_acceptance_metric 1\n# EOF\n"

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path != "/metrics":
                    self.send_response(404)
                    self.end_headers()
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/openmetrics-text; version=1.0.0; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, format, *args):
                return

        def emit_logs():
            while True:
                print("baseharbor-observability-acceptance-api", flush=True)
                time.sleep(1)

        threading.Thread(target=emit_logs, daemon=True).start()
        server = HTTPServer(("0.0.0.0", 8080), Handler)
        cert_file = os.environ.get("TLS_CERT_FILE", "").strip()
        key_file = os.environ.get("TLS_KEY_FILE", "").strip()
        if cert_file and key_file:
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain(certfile=cert_file, keyfile=key_file)
            server.socket = context.wrap_socket(server.socket, server_side=True)
        server.serve_forever()
    healthcheck:
      test: ["CMD", "python", "-c", "import socket; socket.create_connection(('127.0.0.1', 8080), 2).close()"]
      interval: 2s
      timeout: 5s
      retries: 30
  trace-probe:
    image: docker.io/curlimages/curl:8.16.0
    entrypoint: ["sh", "-c"]
    command: ["while :; do echo baseharbor-observability-acceptance-trace-probe; sleep 1; done"]
    healthcheck:
      test:
        - CMD-SHELL
        - >-
          set -- --silent --show-error --max-time 3 --cacert "$$OTEL_EXPORTER_OTLP_CERTIFICATE";
          if [ -n "$${OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE:-}" ]; then
          set -- "$$@" --cert "$$OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE" --key "$$OTEL_EXPORTER_OTLP_CLIENT_KEY";
          fi;
          code=$$(curl "$$@" -o /dev/null -w '%{http_code}' "$${OTEL_EXPORTER_OTLP_ENDPOINT%/}/") &&
          case "$$code" in 200|204|404|405) exit 0;; *) exit 1;; esac
      interval: 2s
      timeout: 5s
      retries: 30
`
	if err := os.WriteFile("compose.yaml", []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	defer func() {
		out.Reset()
		_ = runWithIO(context.Background(), []string{"app", "destroy", "--yes"}, &out, &out)
	}()

	// The provider-change consent guard requires a plan→apply flow.
	// On first apply the impact plan is complete=false → confirmations=2,
	// so a direct "app apply" without consent fails with
	// "impact-confirmation-required". Build the impact plan, provide the
	// required two-stage consent, and carry it through with
	// withProviderChangeConsent so the apply lifecycle can honour the
	// prescribed plan→apply acknowledgement.
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatalf("resolve application for plan build: %v", err)
	}
	plan, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		t.Fatalf("build resolved application plan: %v", err)
	}
	if plan.ProviderPlan.Impact == nil {
		t.Fatal("app apply requires a provider-impact plan")
	}
	impact := plan.ProviderPlan.Impact

	// Construct the two-stage consent required by the impact plan.
	// Complete=false or confirmations==2 both require First+Second.
	consent := &application.ProviderChangeConsent{
		PlanID:     impact.PlanID,
		First:      true,
		Second:     impact.ConfirmationsRequired == 2,
		DeleteData: false,
	}
	if err := application.ValidateProviderChangeConsent(*impact, *consent); err != nil {
		t.Fatalf("impact consent validation failed: %v", err)
	}

	if err := runWithIO(withProviderChangeConsent(ctx, consent), []string{"--verbose", "app", "apply"}, &out, &out); err != nil {
		t.Fatalf("full-stack observability apply failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "application and requested infrastructure verified") {
		t.Fatalf("apply output missing final verified-ready state:\n%s", out.String())
	}
	providerEvidence := regexp.MustCompile(`(?m)^\[VERIFIED\] logs\s+[0-9]+ workload stream\(s\), [1-9][0-9]* provider stream\(s\) ingested for ` + regexp.QuoteMeta(m.Name) + `$`).FindString(out.String())
	if providerEvidence == "" {
		t.Fatalf("apply did not verify ingestion of any provider log streams:\n%s", out.String())
	}
	t.Log(providerEvidence)

	assertSignalProviders(t, m, observability.SignalMetrics, []capability.ProviderKind{
		capability.ProviderOTelCollector,
		capability.ProviderLoki,
		capability.ProviderTempo,
		capability.ProviderRuntimeBroker,
		capability.ProviderRuntimeExecutor,
	})
	assertSignalProviders(t, m, observability.SignalLogs, []capability.ProviderKind{
		capability.ProviderPostgreSQL,
		capability.ProviderValkey,
		capability.ProviderRuntimeBroker,
		capability.ProviderRuntimeExecutor,
	})
	assertSignalProviders(t, m, observability.SignalTraces, []capability.ProviderKind{
		capability.ProviderPostgreSQL,
		capability.ProviderValkey,
		capability.ProviderRuntimeBroker,
		capability.ProviderRuntimeExecutor,
	})

	statusResolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(statusResolved.Store, statusResolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, statusResolved.Manifest, files)
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
curl "$@" -H "Content-Type: application/x-protobuf" --data-binary @- "${OTEL_EXPORTER_OTLP_ENDPOINT%/}/v1/traces"
`
	if _, err := compose.ExecProjectFilesInput(ctx, workload.Project, root, "trace-probe", composeFiles, payload, "sh", "-ec", tracePost); err != nil {
		t.Fatalf("workload OTLP export through injected binding failed: %v", err)
	}
	if err := tracesprovider.VerifyTraceAt(ctx, statusResolved.Manifest, traceID, statusResolved.TargetStateRoot, statusResolved.Target.Name); err != nil {
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
