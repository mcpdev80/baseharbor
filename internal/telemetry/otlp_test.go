package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type noopRuntime struct{}

func (noopRuntime) ConfigProject(context.Context, string, string, string) error  { return nil }
func (noopRuntime) UpProject(context.Context, string, string, string) error      { return nil }
func (noopRuntime) DestroyProject(context.Context, string, string, string) error { return nil }

func TestExternalOTLPVerifySendsRealProtobufTrace(t *testing.T) {
	var contentType string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("path = %s", r.URL.Path)
		}
		contentType = r.Header.Get("Content-Type")
		body = make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv(application.OTLPEndpointEnv, server.URL)
	m := application.WithOTLPTelemetry(application.Manifest{Version: 1, Name: "demo", Environment: "test", Workload: application.WorkloadConfig{Services: []string{"api"}}}, "traces")
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{}, serviceissuer.New(t))
	resource := capability.Resource{Application: "demo", Kind: capability.TelemetryOTLP, Name: "default", Provider: capability.ProviderExternalOTLP}
	if err := d.Verify(context.Background(), resource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if contentType != "application/x-protobuf" {
		t.Fatalf("content-type = %q", contentType)
	}
	if len(body) == 0 || !strings.Contains(string(body), "baseharbor.otlp.verify") {
		t.Fatal("verification request did not contain the BaseHarbor OTLP probe span")
	}
}

func TestExternalOTLPPreflightRejectsInvalidEndpoint(t *testing.T) {
	_ = os.Setenv(application.OTLPEndpointEnv, "ftp://bad.example")
	defer os.Unsetenv(application.OTLPEndpointEnv)
	m := application.WithOTLPTelemetry(application.Manifest{Version: 1, Name: "demo", Environment: "test", Workload: application.WorkloadConfig{Services: []string{"api"}}}, "traces")
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{}, serviceissuer.New(t))
	resource := capability.Resource{Application: "demo", Kind: capability.TelemetryOTLP, Name: "default", Provider: capability.ProviderExternalOTLP}
	binding := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: []string{"traces"}}}
	if err := d.Preflight(context.Background(), resource, binding); err == nil {
		t.Fatal("expected invalid external endpoint to fail closed")
	}
}

func TestManagedCollectorDoesNotProvisionObservabilityBackends(t *testing.T) {
	composeText := strings.ToLower(providerComposeYAML())
	for _, forbidden := range []string{"prom/prometheus", "grafana", "loki", "tempo"} {
		if strings.Contains(composeText, forbidden) {
			t.Fatalf("managed OTLP provider unexpectedly provisions %s:\n%s", forbidden, composeText)
		}
	}
	configText := strings.ToLower(collectorConfig())
	for _, forbidden := range []string{"http://loki", "http://tempo", "grafana"} {
		if strings.Contains(configText, forbidden) {
			t.Fatalf("managed OTLP provider unexpectedly configures backend %s:\n%s", forbidden, configText)
		}
	}
}

func TestExternalProviderSelectionWithoutEndpointFailsClosed(t *testing.T) {
	t.Setenv(application.OTLPProviderEnv, "external")
	t.Setenv(application.OTLPEndpointEnv, "")
	m := application.WithOTLPTelemetry(application.Manifest{
		Version: 1, Name: "demo", Environment: "test",
		Workload: application.WorkloadConfig{Services: []string{"api"}},
	}, "traces")
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{}, serviceissuer.New(t))
	if d.Descriptor().Kind != capability.ProviderExternalOTLP {
		t.Fatalf("descriptor = %#v", d.Descriptor())
	}
	resource := capability.Resource{Application: "demo", Kind: capability.TelemetryOTLP, Name: "default", Provider: capability.ProviderExternalOTLP}
	binding := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: []string{"traces"}}}
	if err := d.Preflight(context.Background(), resource, binding); err == nil {
		t.Fatal("expected missing external endpoint to fail closed")
	}
}

func TestEnsureProviderFilesKeepsRuntimeStatePrivateButCollectorConfigReadable(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t))
	if err != nil {
		t.Fatal(err)
	}

	configInfo, err := os.Stat(files.Config)
	if err != nil {
		t.Fatal(err)
	}
	if got := configInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("collector config mode = %o, want 644", got)
	}

	envInfo, err := os.Stat(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if got := envInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("provider env mode = %o, want 600", got)
	}
}

func TestManagedCollectorRunsUnprivileged(t *testing.T) {
	text := providerComposeYAML()
	for _, want := range []string{
		"user: \"10001:10001\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"/tmp:rw,noexec,nosuid,nodev",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed OTLP provider missing %q:\n%s", want, text)
		}
	}
}

func TestManagedCollectorTraceBackendUsesCanonicalOTLPHTTPExporter(t *testing.T) {
	config := collectorConfigWithTraceBackend("http://tempo:4318")
	for _, want := range []string{
		"otlp_http/tempo:",
		"endpoint: http://tempo:4318",
		"exporters: [debug, otlp_http/tempo]",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("trace backend config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "otlphttp/tempo") {
		t.Fatalf("deprecated otlphttp exporter alias rendered:\n%s", config)
	}
}

func TestManagedCollectorExposesInternalMetricsOnProviderNetwork(t *testing.T) {
	config := collectorConfigWithTraceBackend("")
	for _, want := range []string{
		"telemetry:",
		"metrics:",
		"readers:",
		"pull:",
		"prometheus:",
		"host: 0.0.0.0",
		"port: 8888",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("collector config missing %q:\n%s", want, config)
		}
	}
}

func TestProviderInteractionTracePayloadIsAttributedAndUnique(t *testing.T) {
	m := application.Manifest{Version: 1, Name: "demo", Environment: "dev"}
	source := observability.SignalSource{
		ID:                 "postgresql:demo:postgres",
		Kind:               observability.SignalTraces,
		Provider:           capability.ProviderPostgreSQL,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   "demo",
		Target:             observability.RuntimeTarget("baseharbor-demo-dev", "postgres"),
		Protocol:           "interaction",
		Mode:               capability.ObservabilityInteraction,
		SemanticConvention: "database",
		Verification:       capability.ObservabilityVerifySpan,
	}

	first, firstID := providerInteractionTracePayload(m, source)
	second, secondID := providerInteractionTracePayload(m, source)
	if firstID == secondID {
		t.Fatalf("provider verification trace ids are not unique: %s", firstID)
	}
	for _, want := range []string{
		"baseharbor.provider.interaction.verify",
		"postgresql",
		"postgresql:demo:postgres",
		"baseharbor-demo-dev/postgres",
		"database",
		"demo",
		"dev",
	} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("provider interaction trace missing %q", want)
		}
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("provider interaction trace payload is empty")
	}
}
