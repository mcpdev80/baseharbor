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
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{})
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
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{})
	resource := capability.Resource{Application: "demo", Kind: capability.TelemetryOTLP, Name: "default", Provider: capability.ProviderExternalOTLP}
	binding := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: []string{"traces"}}}
	if err := d.Preflight(context.Background(), resource, binding); err == nil {
		t.Fatal("expected invalid external endpoint to fail closed")
	}
}

func TestManagedCollectorDoesNotProvisionObservabilityBackends(t *testing.T) {
	text := strings.ToLower(providerComposeYAML() + "\n" + collectorConfig())
	for _, forbidden := range []string{"prometheus", "loki", "tempo", "grafana"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("managed OTLP provider unexpectedly references %s", forbidden)
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
	d := NewDriver(noopRuntime{}, m, application.RuntimeFiles{})
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
	files, err := EnsureProviderFiles()
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
