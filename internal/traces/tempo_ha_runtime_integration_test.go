package traces_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

func TestTempoHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEMPO_HA_ACCEPTANCE") != "1" {
		t.Skip("Tempo HA acceptance requires BASEHARBOR_TEMPO_HA_ACCEPTANCE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	runtime, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	namespace := "tempo-ha-acceptance"
	t.Setenv(application.TracesEnabledEnv, "true")

	m := application.WithOTLPTelemetry(application.New("tempo-ha-ci", "dev", false, false, false), "traces")
	m.HA = true
	issuer := serviceissuer.New(t)

	traceResource := capability.Resource{
		Application: m.Name,
		Kind:        capability.Traces,
		Name:        "default",
		Provider:    capability.ProviderTempo,
	}
	traceDriver := traces.NewDriverAt(runtime, m, issuer, dataDir, namespace)
	if err := traceDriver.Preflight(ctx, traceResource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	provisionCtx, provisionCancel := context.WithTimeout(ctx, 3*time.Minute)
	err = traceDriver.Provision(provisionCtx, traceResource, capability.Binding{})
	provisionCancel()
	if err != nil {
		detail := ""
		if placement, placementErr := traces.PlacementForAt(dataDir, namespace, m); placementErr == nil {
			if files, _, filesErr := traces.ExistingProviderFilesAt(dataDir, namespace, m); filesErr == nil {
				diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 15*time.Second)
				detail = strings.TrimSpace(runtime.DiagnosticsProject(diagnosticCtx, placement.Project, files.Compose, files.Env))
				diagnosticCancel()
			}
		}
		if detail != "" {
			t.Fatalf("provision Tempo HA: %v\n%s", err, detail)
		}
		t.Fatalf("provision Tempo HA: %v", err)
	}
	defer func() {
		_ = traces.DestroyAllSharedProvidersAt(context.Background(), runtime, dataDir, namespace)
	}()

	placement, err := traces.PlacementForAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := traces.ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	env := readTempoHAEnv(t, files.Env)

	otelDriver := telemetry.NewDriverAt(runtime, m, application.RuntimeFiles{}, issuer, dataDir, namespace)
	otelDriver.SetTraceBackend(traces.NetworkEndpoint(placement), placement.Network)
	otelResource := capability.Resource{
		Application: m.Name,
		Kind:        capability.TelemetryOTLP,
		Name:        "default",
		Provider:    capability.ProviderOTelCollector,
	}
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
		t.Fatalf("provision HA OTel transport: %v", err)
	}
	defer func() {
		_ = telemetry.DestroySharedProviderAt(context.Background(), runtime, dataDir, namespace)
	}()

	verifyTempoHAIngestionAndQuery(t, ctx, otelDriver, otelResource, otelBinding, m, dataDir, namespace)

	for _, member := range []struct {
		name          string
		requireIngest bool
	}{
		{name: "tempo-query-frontend-1"},
		{name: "tempo-distributor-1", requireIngest: true},
		{name: "redpanda-1", requireIngest: true},
	} {
		if err := runtime.StopProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{member.name}, files.Compose); err != nil {
			t.Fatalf("stop %s: %v", member.name, err)
		}

		if member.requireIngest {
			verifyTempoHAIngestionAndQuery(t, ctx, otelDriver, otelResource, otelBinding, m, dataDir, namespace)
		} else {
			waitTempoHAQuery(t, ctx, m, dataDir, namespace)
		}

		if err := runtime.UpProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{member.name}, files.Compose); err != nil {
			t.Fatalf("restart %s: %v", member.name, err)
		}
		verifyTempoHAIngestionAndQuery(t, ctx, otelDriver, otelResource, otelBinding, m, dataDir, namespace)
	}
}

func verifyTempoHAIngestionAndQuery(
	t *testing.T,
	ctx context.Context,
	otelDriver interface {
		Verify(context.Context, capability.Resource, capability.Binding) error
	},
	resource capability.Resource,
	binding capability.Binding,
	m application.Manifest,
	dataDir, namespace string,
) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if err := otelDriver.Verify(ctx, resource, binding); err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		if err := traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, dataDir, namespace); err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		return
	}
	t.Fatalf("Tempo HA ingestion/query continuity failed: %v", last)
}

func waitTempoHAQuery(t *testing.T, ctx context.Context, m application.Manifest, dataDir, namespace string) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if last = traces.VerifyTraceAt(ctx, m, telemetry.ProbeTraceIDHex, dataDir, namespace); last == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("Tempo HA query continuity failed: %v", last)
}

func readTempoHAEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid Tempo env line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}
