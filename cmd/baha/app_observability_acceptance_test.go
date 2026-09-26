package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
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
	m = application.WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")
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

        print("baseharbor-observability-acceptance-api", flush=True)
        server = HTTPServer(("0.0.0.0", 8080), Handler)
        cert_file = os.environ.get("TLS_CERT_FILE", "").strip()
        key_file = os.environ.get("TLS_KEY_FILE", "").strip()
        if cert_file and key_file:
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain(certfile=cert_file, keyfile=key_file)
            server.socket = context.wrap_socket(server.socket, server_side=True)
        server.serve_forever()
  trace-probe:
    image: docker.io/curlimages/curl:8.16.0
    entrypoint: ["sh", "-c"]
    command: ["echo baseharbor-observability-acceptance-trace-probe; sleep 3600"]
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
		diag := ""
		if compose.Engine() == "docker" {
			resolved, resolveErr := resolveApplication(ctx, application.Store{}, nil, "status")
			if resolveErr != nil {
				diag = "resolve diagnostic state: " + resolveErr.Error()
			} else {
				registration, registrationErr := logsprovider.ApplicationRegistrationAt(resolved.TargetStateRoot, resolved.Target.Name, m)
				if registrationErr != nil {
					diag = "load log registration: " + registrationErr.Error()
				} else {
					placement, placementErr := application.ResolveProviderPlacement(m, capability.ProviderLoki)
					if placementErr != nil {
						diag = "resolve Loki placement: " + placementErr.Error()
					} else {
						sources, listErr := observability.List(observability.SignalLogs, placement, []string{m.Name}, true, true)
						if listErr != nil {
							diag = "list provider log sources: " + listErr.Error()
						} else {
							var brokerSources []observability.SignalSource
							for _, source := range sources {
								if source.Provider == capability.ProviderRuntimeBroker && source.Class == observability.SourceApplicationProvider {
									brokerSources = append(brokerSources, source)
								}
							}

							pid1Result := "not-run"
							files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
							if filesErr != nil {
								pid1Result = "runtime-files=" + filesErr.Error()
							} else {
								brokerFiles, brokerErr := runtimebroker.Existing(files)
								if brokerErr != nil {
									pid1Result = "broker-files=" + brokerErr.Error()
								} else {
									_, execErr := compose.ExecProject(
										context.Background(),
										runtimebroker.ProjectNameForRuntime(m, files),
										brokerFiles.Compose,
										files.Env,
										runtimebroker.ServiceName,
										"sh",
										"-ec",
										"printf '%s\\n' pid1-stdout-provider-log > /proc/1/fd/1",
									)
									if execErr != nil {
										pid1Result = "write=" + execErr.Error()
									} else {
										verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 12*time.Second)
										verifyErr := logsprovider.VerifyProviderSourcesAt(verifyCtx, m, brokerSources, resolved.TargetStateRoot, resolved.Target.Name)
										verifyCancel()
										pid1Result = fmt.Sprintf("verify=%v", verifyErr)
									}
								}
							}

							conn, dialErr := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", registration.ProviderSyslogPort))
							directResult := ""
							if dialErr != nil {
								directResult = "dial=" + dialErr.Error()
							} else {
								_, writeErr := fmt.Fprintf(conn, "<14>1 %s baseharbor runtime-broker/baseharbor-internal-broker - - - synthetic-broker-provider-log\\n", time.Now().UTC().Format(time.RFC3339))
								_ = conn.Close()
								if writeErr != nil {
									directResult = "write=" + writeErr.Error()
								} else {
									verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 20*time.Second)
									verifyErr := logsprovider.VerifyProviderSourcesAt(verifyCtx, m, brokerSources, resolved.TargetStateRoot, resolved.Target.Name)
									verifyCancel()
									directResult = fmt.Sprintf("verify=%v", verifyErr)
								}
							}
							diag = fmt.Sprintf("provider port=%d brokerSources=%d pid1-stdout[%s] direct-udp[%s]", registration.ProviderSyslogPort, len(brokerSources), pid1Result, directResult)
						}
					}
				}
			}
		}
		t.Fatalf("full-stack observability apply failed: %v\nDIAGNOSTIC: %s\n%s", err, diag, out.String())
	}
	if !strings.Contains(out.String(), "application and requested infrastructure verified") {
		t.Fatalf("apply output missing final verified-ready state:\n%s", out.String())
	}

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

	resolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, resolved.Manifest, files)
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
	if err := tracesprovider.VerifyTraceAt(ctx, resolved.Manifest, traceID, resolved.TargetStateRoot, resolved.Target.Name); err != nil {
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
