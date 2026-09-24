package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestProviderFilesUsePinnedPrometheusAndHardenedSharedNetwork(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), application.Manifest{Name: "demo", Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, want := range []string{
		"image: " + ProviderImage,
		"127.0.0.1:${BASEHARBOR_PROMETHEUS_PORT}:8443",
		"access:\n    internal: true",
		"publish: {}",
		"read_only: true",
		"cap_drop:",
		"- ALL",
		"no-new-privileges:true",
		"name: " + strconv.Quote(application.MetricsProviderNetworkName(application.Manifest{Name: "demo", Environment: "dev"})),
		"name: " + strconv.Quote("baseharbor-prometheus-data"),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Prometheus compose missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"grafana", "loki", "tempo", "/var/run/docker.sock"} {
		if strings.Contains(strings.ToLower(text), unwanted) {
			t.Fatalf("Prometheus compose unexpectedly contains %q:\n%s", unwanted, text)
		}
	}

	config, err := os.ReadFile(files.Config)
	if err != nil {
		t.Fatal(err)
	}
	configText := string(config)
	if !strings.Contains(configText, "file_sd_configs:") ||
		!strings.Contains(configText, "/etc/prometheus/targets/*--*--*.json") ||
		!strings.Contains(configText, "target_label: __metrics_path__") ||
		!strings.Contains(configText, "target_label: __scheme__") {
		t.Fatalf("Prometheus config does not use dynamic file discovery/path relabeling:\n%s", configText)
	}
}

func TestBindWritesAttributedTargetAndPrunesOnlySameApplication(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	if _, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), application.Manifest{Name: "demo", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}

	alpha := application.New("alpha", "dev", false, false, false)
	alpha.Services.SQL = false
	alpha = application.WithWorkload(alpha, "compose.yaml", "api")
	alpha = application.WithMetricsSource(alpha, "application", "api", 8080, "/metrics")

	beta := alpha
	beta.Name = "beta"

	bind := func(m application.Manifest) {
		t.Helper()
		driver := NewDriver(nil, m, serviceissuer.New(t))
		resource := capability.Resource{
			Application: m.Name,
			Kind:        capability.Metrics,
			Name:        "application",
			Provider:    capability.ProviderPrometheus,
		}
		binding := capability.Binding{
			Resource: resource,
			Workload: "service/api",
			Metrics: &capability.MetricsBinding{
				Direction: "provide",
				Format:    "openmetrics",
				Service:   "api",
				Scheme:    "https",
				Port:      8080,
				Path:      "/metrics",
			},
		}
		if err := driver.Bind(context.Background(), resource, binding); err != nil {
			t.Fatal(err)
		}
	}

	bind(alpha)
	bind(beta)

	files, err := ExistingProviderFiles(alpha)
	if err != nil {
		t.Fatal(err)
	}
	alphaPath := filepath.Join(files.TargetsDir, targetFileName(alpha, "application"))
	data, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	var groups []targetGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Targets) != 1 {
		t.Fatalf("unexpected target groups %#v", groups)
	}
	if groups[0].Targets[0] != application.MetricsTargetAlias(alpha, "api")+":8080" {
		t.Fatalf("target = %q", groups[0].Targets[0])
	}
	labels := groups[0].Labels
	if labels["baseharbor_application"] != "alpha" ||
		labels["baseharbor_environment"] != "dev" ||
		labels["baseharbor_service"] != "api" ||
		labels["baseharbor_source"] != "application" ||
		labels["baseharbor_metrics_path"] != "/metrics" ||
		labels["baseharbor_metrics_scheme"] != "https" {
		t.Fatalf("labels = %#v", labels)
	}

	if err := PruneApplicationTargets(alpha, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alphaPath); !os.IsNotExist(err) {
		t.Fatalf("alpha target still exists: %v", err)
	}
	betaPath := filepath.Join(files.TargetsDir, targetFileName(beta, "application"))
	if _, err := os.Stat(betaPath); err != nil {
		t.Fatalf("beta target was removed while pruning alpha: %v", err)
	}
}

func TestSharedProviderUsesSeparateNetworkPerApplication(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")
	t.Setenv(application.MetricsEnabledEnv, "true")

	alpha := application.New("alpha", "dev", false, false, false)
	alpha.Services.SQL = false
	alpha = application.WithWorkload(alpha, "compose.yaml", "api")
	alpha = application.WithMetricsSource(alpha, "application", "api", 8080, "/metrics")

	beta := alpha
	beta.Name = "beta"

	if _, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), alpha); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), beta)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	alphaNetwork := application.MetricsProviderNetworkName(alpha)
	betaNetwork := application.MetricsProviderNetworkName(beta)
	if alphaNetwork == betaNetwork {
		t.Fatalf("metrics networks collide: %q", alphaNetwork)
	}
	for _, network := range []string{alphaNetwork, betaNetwork} {
		if !strings.Contains(text, "name: "+strconv.Quote(network)) {
			t.Fatalf("shared provider missing isolated network %q:\n%s", network, text)
		}
	}
	if strings.Contains(text, "name: \"baseharbor-metrics\"") {
		t.Fatalf("shared provider reintroduced flat application metrics network:\n%s", text)
	}
}

func TestApplicationScopedPlacementIsIsolated(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")
	t.Setenv(application.MetricsEnabledEnv, "true")

	alpha := application.New("alpha", "dev", false, false, false)
	beta := application.New("beta", "dev", false, false, false)

	a, err := PlacementFor(alpha)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PlacementFor(beta)
	if err != nil {
		t.Fatal(err)
	}
	if a.Project == b.Project || a.Network == b.Network || a.Volume == b.Volume || a.Dir == b.Dir {
		t.Fatalf("application-scoped placements are not isolated: alpha=%#v beta=%#v", a, b)
	}
	if a.Scope != capability.ScopeApplication || b.Scope != capability.ScopeApplication {
		t.Fatalf("unexpected scopes: alpha=%s beta=%s", a.Scope, b.Scope)
	}
}

func TestSharedPlacementBoundaryGetsIndependentProviderState(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")

	m := application.New("alpha", "dev", false, false, false)
	t.Setenv(application.ProviderSharingBoundaryEnv(capability.ProviderPrometheus), "team-a")
	a, err := PlacementFor(m)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(application.ProviderSharingBoundaryEnv(capability.ProviderPrometheus), "team-b")
	b, err := PlacementFor(m)
	if err != nil {
		t.Fatal(err)
	}

	if a.Project == b.Project || a.Volume == b.Volume || a.Dir == b.Dir {
		t.Fatalf("sharing boundaries are not isolated: a=%#v b=%#v", a, b)
	}
	if a.Scope != capability.ScopeShared || b.Scope != capability.ScopeShared {
		t.Fatalf("unexpected scopes: a=%s b=%s", a.Scope, b.Scope)
	}
}

type recordingRuntime struct {
	destroyed    []string
	missingState bool
}

func (r *recordingRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (r *recordingRuntime) UpProject(context.Context, string, string, string) error     { return nil }
func (r *recordingRuntime) DownProject(context.Context, string, string, string) error   { return nil }
func (r *recordingRuntime) DestroyProject(_ context.Context, project, composeFile, envFile string) error {
	for _, path := range []string{composeFile, envFile} {
		if _, err := os.Stat(path); err != nil {
			r.missingState = true
		}
	}
	r.destroyed = append(r.destroyed, project)
	return nil
}

func TestDestroyAllSharedProvidersIncludesSharingBoundaries(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")

	m := application.New("alpha", "dev", false, false, false)
	for _, boundary := range []string{"", "team-a", "team-b"} {
		t.Setenv(application.ProviderSharingBoundaryEnv(capability.ProviderPrometheus), boundary)
		if _, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), m); err != nil {
			t.Fatal(err)
		}
	}

	instances, err := ExistingSharedProviderInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 3 {
		t.Fatalf("shared provider instances=%d want=3: %#v", len(instances), instances)
	}

	runtime := &recordingRuntime{}
	if err := DestroyAllSharedProviders(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	if len(runtime.destroyed) != 3 {
		t.Fatalf("destroyed projects=%#v", runtime.destroyed)
	}
	if runtime.missingState {
		t.Fatal("shared Prometheus state was removed before all provider projects were destroyed")
	}
	instances, err = ExistingSharedProviderInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 0 {
		t.Fatalf("shared provider state remains: %#v", instances)
	}
}

func TestPrometheusConfigUsesExplicitRuntimeTargetDirectories(t *testing.T) {
	config := prometheusConfig([]sourceRegistration{
		{Application: "alpha", Environment: "dev", RuntimeVolume: "runtime-alpha"},
		{Application: "beta", Environment: "dev"},
		{Application: "gamma", Environment: "dev", RuntimeVolume: "runtime-gamma"},
	}, false)
	for _, want := range []string{
		"/etc/prometheus/targets/*--*--*.json",
		"/etc/prometheus/runtime-targets/0/*.json",
		"/etc/prometheus/runtime-targets/2/*.json",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("Prometheus config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "/etc/prometheus/runtime-targets/*/*.json") {
		t.Fatalf("Prometheus config contains unsupported nested wildcard file discovery path:\n%s", config)
	}
	if strings.Contains(config, "/etc/prometheus/runtime-targets/1/*.json") {
		t.Fatalf("Prometheus config rendered runtime target path for registration without runtime volume:\n%s", config)
	}
}

func TestProviderFilesTrustManagedRuntimeCAForHTTPSMetrics(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")

	caSource := filepath.Join(t.TempDir(), "ca.pem")
	const caData = "test-runtime-ca"
	if err := os.WriteFile(caSource, []byte(caData), 0o600); err != nil {
		t.Fatal(err)
	}

	m := application.New("demo", "dev", false, false, false)
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	files, err := EnsureProviderFilesWithRuntimeCA(context.Background(), serviceissuer.New(t), m, caSource)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := os.ReadFile(files.RuntimeCA)
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != caData {
		t.Fatalf("runtime CA = %q, want %q", copied, caData)
	}

	config, err := os.ReadFile(files.Config)
	if err != nil {
		t.Fatal(err)
	}
	configText := string(config)
	for _, want := range []string{
		"tls_config:",
		"ca_file: /etc/prometheus/baseharbor-runtime-ca.pem",
		"target_label: __scheme__",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("Prometheus config missing %q:\n%s", want, configText)
		}
	}

	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "./baseharbor-runtime-ca.pem:/etc/prometheus/baseharbor-runtime-ca.pem:ro") {
		t.Fatalf("Prometheus compose does not mount runtime CA:\n%s", compose)
	}
}


func TestProviderComposeKeepsGatewayOnRuntimeProjectedTLSMaterial(t *testing.T) {
	rendered := providerComposeYAMLWithProviderNetworks(
		Placement{Scope: capability.ScopeShared, Project: "baseharbor-metrics", Volume: "baseharbor-prometheus-data"},
		nil,
		nil,
		false,
	)
	for _, want := range []string{
		"./service-access/runtime/ca.pem:/certs/ca.pem:ro",
		"./service-access/runtime/server.pem:/certs/server.pem:ro",
		"./service-access/runtime/server-key.pem:/certs/server-key.pem:ro",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Prometheus gateway compose missing runtime-projected TLS material %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{
		"./service-access/pki/ca.pem:/certs/ca.pem:ro",
		"./service-access/pki/server-cert.pem:/certs/server.pem:ro",
		"./service-access/pki/server-key.pem:/certs/server-key.pem:ro",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Prometheus gateway compose mounted protected PKI source material %q:\n%s", forbidden, rendered)
		}
	}
}
