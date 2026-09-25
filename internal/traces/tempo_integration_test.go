package traces_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

func TestManagedTempoReceivesVerificationTraceThroughCollector(t *testing.T) {
	if os.Getenv("BASEHARBOR_TRACES_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_TRACES_INTEGRATION=1 for real Docker/Podman Tempo acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	providerState := t.TempDir()
	namespace := "trace-acceptance"
	t.Setenv(application.TracesEnabledEnv, "true")

	m := application.WithOTLPTelemetry(application.New("traces-acceptance", "dev", false, false, false), "traces")
	issuer := serviceissuer.New(t)

	traceResource := capability.Resource{Application: m.Name, Kind: capability.Traces, Name: "default", Provider: capability.ProviderTempo}
	traceDriver := traces.NewDriverAt(compose, m, issuer, providerState, namespace)
	if err := traceDriver.Preflight(ctx, traceResource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := traceDriver.Provision(ctx, traceResource, capability.Binding{}); err != nil {
		t.Fatalf("provision Tempo: %v", err)
	}
	defer func() { _ = traces.DestroyAllSharedProvidersAt(context.Background(), compose, providerState, namespace) }()

	placement, err := traces.PlacementForAt(providerState, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := containersecurity.VerifyComposeService(ctx, placement.Project, "tempo", containersecurity.Requirements{
		ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
	}); err != nil {
		t.Fatalf("Tempo runtime security: %v", err)
	}

	otelDriver := telemetry.NewDriverAt(compose, m, application.RuntimeFiles{}, issuer, providerState, namespace)
	otelDriver.SetTraceBackend(traces.NetworkEndpoint(placement), placement.Network)
	otelResource := capability.Resource{Application: m.Name, Kind: capability.TelemetryOTLP, Name: "default", Provider: capability.ProviderOTelCollector}
	otelBinding := capability.Binding{
		Resource: otelResource,
		Workload: "application",
		TelemetryOTLP: &capability.OTLPTelemetryBinding{
			Direction: "export",
			Protocol:  "http/protobuf",
			Signals:   []string{"traces"},
		},
	}
	if err := otelDriver.Preflight(ctx, otelResource, otelBinding); err != nil {
		t.Fatal(err)
	}
	if err := otelDriver.Provision(ctx, otelResource, otelBinding); err != nil {
		t.Fatalf("provision OpenTelemetry Collector: %v", err)
	}
	defer func() { _ = telemetry.DestroySharedProviderAt(context.Background(), compose, providerState, namespace) }()

	files, err := telemetry.ExistingProviderFilesAt(providerState, namespace)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := telemetry.ProviderEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/traces", bytes.NewReader(telemetry.VerificationTracePayload(m)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("OTLP verification export returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, providerState, namespace); err != nil {
		t.Fatalf("query verification trace from Tempo: %v", err)
	}
	fmt.Println("verified OTLP Collector -> Tempo trace ingestion")
}
