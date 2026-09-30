package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func telemetryManifest() Manifest {
	return WithOTLPTelemetry(Manifest{
		Version: 1, Name: "demo", Environment: "dev",
		Workload: WorkloadConfig{Services: []string{"api"}},
	}, "traces", "metrics")
}

func TestOTLPTelemetryManifestRoundTrip(t *testing.T) {
	m := telemetryManifest()
	data := m.YAML()
	got, err := ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Telemetry.OTLP == nil || strings.Join(got.Telemetry.OTLP.Signals, ",") != "metrics,traces" {
		t.Fatalf("telemetry = %#v\nyaml:\n%s", got.Telemetry, data)
	}
	contract, err := PortableContractFromManifest(got)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, requirement := range contract.Capabilities {
		if requirement.Kind == capability.TelemetryOTLP {
			found = true
		}
	}
	if !found {
		t.Fatal("portable contract is missing telemetry.otlp")
	}
}

func TestOTLPTelemetryRequiresExplicitWorkloadIdentity(t *testing.T) {
	m := telemetryManifest()
	m.Workload.Services = nil
	if err := m.Validate(); err == nil {
		t.Fatal("expected deterministic workload identity validation error")
	}
}

func TestMaterializeOTLPBindingUsesStandardOpenTelemetryEnvironment(t *testing.T) {
	dir := t.TempDir()
	files := RuntimeFiles{
		Dir:            dir,
		Env:            filepath.Join(dir, "runtime.env"),
		ApplicationEnv: filepath.Join(dir, "application.env"),
		Bindings:       filepath.Join(dir, "bindings"),
	}
	if err := os.WriteFile(files.Env, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ApplicationEnv, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m := telemetryManifest()
	if err := MaterializeOTLPBinding(m, files, capability.ProviderOTelCollector, "http://127.0.0.1:4318", "http://otel-collector:4318"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318",
		"OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		"OTEL_SERVICE_NAME=demo",
		"deployment.environment.name=dev",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
}

func TestTelemetryProviderExternalIsDeploymentState(t *testing.T) {
	t.Setenv(OTLPEndpointEnv, "https://otel.example.test")
	if got := TelemetryProviderForDeployment(); got.Kind != capability.ProviderExternalOTLP {
		t.Fatalf("provider = %#v", got)
	}
}
