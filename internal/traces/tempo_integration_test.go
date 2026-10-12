package traces_test

import (
	"bytes"
	"context"
	"fmt"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

func TestManagedTempoReceivesVerificationTraceThroughCollector(t *testing.T) {
	if os.Getenv("BASEHARBOR_TRACES_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_TRACES_INTEGRATION=1 for real Docker/Podman Tempo acceptance")
	}
	budget := 3 * time.Minute
	if os.Getenv("BASEHARBOR_STANDALONE_RETIREMENT_ACCEPTANCE") == "1" {
		// Tempo 3.0.2 has a five-minute live-store flush operation budget.
		// Preserve the exact original-record assertion while allowing a
		// controlled shutdown; never replace it with forced container removal.
		budget = 8 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	providerState := t.TempDir()
	namespace := "trace-acceptance"
	t.Setenv(application.TracesEnabledEnv, "true")

	environment := "dev"
	if os.Getenv("BASEHARBOR_STANDALONE_RETIREMENT_ACCEPTANCE") == "1" {
		environment = "prod"
	}
	m := application.WithOTLPTelemetry(application.New("traces-acceptance", environment, false, false, false), "traces")
	issuer := serviceissuer.New(t)

	traceResource := capability.Resource{Application: m.Name, Kind: capability.Traces, Name: "default", Provider: capability.ProviderTempo}
	traceDriver := traces.NewDriverAt(compose, m, issuer, providerState, namespace)
	if err := traceDriver.Preflight(ctx, traceResource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := traceDriver.Provision(ctx, traceResource, capability.Binding{}); err != nil {
		t.Fatalf("provision Tempo: %v", err)
	}
	defer func() {
		_ = traces.DestroyAllSharedProvidersAt(context.Background(), compose, providerState, namespace)
	}()

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

	if err := otelDriver.Verify(ctx, otelResource, otelBinding); err != nil {
		t.Fatalf("export OTLP verification trace: %v", err)
	}

	if err := traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, providerState, namespace); err != nil {
		t.Fatalf("query verification trace from Tempo: %v", err)
	}
	if os.Getenv("BASEHARBOR_STANDALONE_RETIREMENT_ACCEPTANCE") == "1" {
		native, ok := compose.(bhruntime.RuntimeServiceRetirementProvider)
		if !ok {
			t.Fatal("selected native retirement adapter unavailable")
		}
		traceFiles, _, err := traces.ExistingProviderFilesAt(providerState, namespace, m)
		if err != nil {
			t.Fatal(err)
		}
		collectorFiles, err := telemetry.ExistingProviderFilesAt(providerState, namespace)
		if err != nil {
			t.Fatal(err)
		}
		traceEnv, _ := os.ReadFile(traceFiles.Env)
		collectorEnv, _ := os.ReadFile(collectorFiles.Env)
		if _, err := telemetry.RetireRuntimeAt(ctx, native, providerState, namespace, placement.Network); err != nil {
			t.Fatal(err)
		}
		if err := traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, providerState, namespace); err != nil {
			t.Fatalf("collector retirement affected retained Tempo sibling: %v", err)
		}
		selected := capability.ProviderPlacement{Scope: placement.Scope, Ownership: capability.OwnershipBaseHarbor}
		if _, err := traces.RetireRuntimeAt(ctx, native, providerState, namespace, m, selected); err != nil {
			diagnostics, done := context.WithTimeout(context.Background(), 10*time.Second)
			t.Log(compose.DiagnosticsProject(diagnostics, placement.Project, traceFiles.Compose, traceFiles.Env))
			done()
			t.Fatal(err)
		}
		if err := compose.UpProject(ctx, placement.Project, traceFiles.Compose, traceFiles.Env); err != nil {
			t.Fatal(err)
		}
		// Read the ORIGINAL trace before any Collector restart/export. No
		// post-retirement Probe/Verify may seed the record being checked.
		if err := traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, providerState, namespace); err != nil {
			t.Fatalf("original authenticated trace lost: %v", err)
		}
		if err := compose.UpProject(ctx, placement.Project, collectorFiles.Compose, collectorFiles.Env); err != nil {
			t.Fatal(err)
		}
		currentTraceEnv, _ := os.ReadFile(traceFiles.Env)
		currentCollectorEnv, _ := os.ReadFile(collectorFiles.Env)
		if !bytes.Equal(traceEnv, currentTraceEnv) || !bytes.Equal(collectorEnv, currentCollectorEnv) {
			t.Fatal("retirement/resume changed retained credentials/configuration")
		}
		t.Log("original trace read after selected Collector and Tempo retirement/resume without reseeding; sibling and retained environment preserved")
	}
	fmt.Println("verified OTLP Collector -> Tempo trace ingestion")
}
