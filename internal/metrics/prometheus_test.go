package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestProviderComposeYAMLIsValidYAML(t *testing.T) {
	placement := Placement{
		Scope:   capability.ScopeShared,
		Project: "bh-local-shared",
		Volume:  "baseharbor-prometheus-data",
	}
	registrations := []sourceRegistration{
		{Application: "demo", Environment: "dev", Network: "baseharbor-demo-dev-metrics"},
	}
	text := providerComposeYAMLWithProviderNetworks(
		placement,
		registrations,
		[]string{"baseharbor-local-telemetry"},
		false,
	)

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("Prometheus compose is invalid YAML: %v\n%s", err, text)
	}
}

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
	webConfigInfo, err := os.Stat(files.WebConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got := webConfigInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("Prometheus web config mode = %o, want 644", got)
	}

	text := string(compose)
	for _, want := range []string{
		"image: " + ProviderImage,
		"127.0.0.1:${BASEHARBOR_PROMETHEUS_PORT}:9090",
		"access:\n    internal: true",
		"publish:\n    name: ",
		"prometheus-access",
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
	alpha = application.WithWorkloadComponents(alpha, "api")
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
	alpha = application.WithWorkloadComponents(alpha, "api")
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

	if a.Project != b.Project || a.Project != "bh-local-shared" {
		t.Fatalf("sharing boundaries must share operator-visible project: a=%#v b=%#v", a, b)
	}
	if a.Volume == b.Volume || a.Dir == b.Dir {
		t.Fatalf("sharing boundary state is not isolated: a=%#v b=%#v", a, b)
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

func TestProviderComposeUsesNativeTLSMaterial(t *testing.T) {
	rendered := providerComposeYAMLWithProviderNetworks(
		Placement{Scope: capability.ScopeShared, Project: "baseharbor-metrics", Volume: "baseharbor-prometheus-data"},
		nil,
		nil,
		false,
	)
	for _, want := range []string{
		"prometheus-1:",
		"prometheus-2:",
		"--web.config.file=/etc/prometheus/web-config.yml",
		"./web-config.yml:/etc/prometheus/web-config.yml:ro",
		"./members/service-access/runtime:/run/baseharbor/tls:ro",
		"prometheus-access:",
		"127.0.0.1:${BASEHARBOR_PROMETHEUS_PORT}:9090",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Prometheus HA compose missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"/service-access/pki/"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Prometheus HA compose contains authority material %q:\n%s", forbidden, rendered)
		}
	}
}

func TestUnregisterSharedApplicationReconcilesServiceAccessProjection(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")

	alpha := application.New("alpha", "dev", false, false, false)
	alpha.Services.SQL = false
	alpha = application.WithWorkloadComponents(alpha, "api")
	alpha = application.WithMetricsSource(alpha, "application", "api", 8080, "/metrics")

	beta := alpha
	beta.Name = "beta"

	issuer := serviceissuer.New(t)
	if _, err := EnsureProviderFiles(context.Background(), issuer, alpha); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureProviderFiles(context.Background(), issuer, beta)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistry(alpha); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistry(beta); err != nil {
		t.Fatal(err)
	}

	runtimeDir := filepath.Join(files.Dir, "members", "service-access", "runtime")
	if err := os.RemoveAll(runtimeDir); err != nil {
		t.Fatal(err)
	}

	runtime := &recordingRuntime{}
	if err := UnregisterSharedApplication(context.Background(), runtime, issuer, alpha); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"ca.pem", "server.pem", "server-key.pem"} {
		if _, err := os.Stat(filepath.Join(runtimeDir, name)); err != nil {
			t.Fatalf("service access runtime projection %s was not reconciled: %v", name, err)
		}
	}

	registrations, err := readRegistrations(files.Registrations)
	if err != nil {
		t.Fatal(err)
	}
	if len(registrations) != 1 || registrations[0].Application != "beta" {
		t.Fatalf("registrations after unregister = %#v, want only beta", registrations)
	}

	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, want := range []string{
		"prometheus-1:",
		"prometheus-2:",
		"/members/service-access/runtime:/run/baseharbor/tls:ro",
		"--web.config.file=/etc/prometheus/web-config.yml",
		"prometheus-access:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reconciled shared Prometheus HA compose missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"/service-access/pki/"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("reconciled shared Prometheus compose contains authority material %q:\n%s", forbidden, text)
		}
	}
}

func TestPrometheusHAFrontendUsesBindMountedMemberTrust(t *testing.T) {
	root := t.TempDir()
	placement := Placement{
		Scope:   capability.ScopeShared,
		Project: "bh-prometheus-ha-test",
		Network: "bh-prometheus-ha-test-network",
		Volume:  "bh-prometheus-ha-test-data",
		Dir:     root,
	}
	access := serviceaccess.HTTPGatewayFiles{
		Caddyfile: filepath.Join(root, "service-access", "config", "Caddyfile"),
		Material: serviceaccess.TLSMaterial{
			CA:                filepath.Join(root, "service-access", "pki", "ca.pem"),
			ServerCertificate: filepath.Join(root, "service-access", "pki", "server.pem"),
			ServerKey:         filepath.Join(root, "service-access", "pki", "server-key.pem"),
			ServerName:        "prometheus",
		},
	}
	got := providerComposeYAMLWithProviderNetworksAndAccess(placement, nil, nil, false, false, access)
	want := filepath.Join(root, "members", "service-access", "runtime") + ":/upstream:ro"
	if !strings.Contains(got, want) {
		t.Fatalf("Prometheus HA frontend does not bind member trust directory %q:\n%s", want, got)
	}
	if strings.Contains(got, "      - members/service-access/runtime:/upstream:ro") {
		t.Fatalf("Prometheus HA frontend rendered member trust as named volume:\n%s", got)
	}
}
