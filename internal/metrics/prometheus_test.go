package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProviderFilesUsePinnedPrometheusAndHardenedSharedNetwork(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	files, err := EnsureProviderFiles()
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
		"127.0.0.1:\${BASEHARBOR_PROMETHEUS_PORT}:9090",
		"read_only: true",
		"cap_drop:",
		"- ALL",
		"no-new-privileges:true",
		"name: baseharbor-metrics",
		"name: baseharbor-prometheus-data",
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
		!strings.Contains(configText, "/etc/prometheus/targets/*.json") ||
		!strings.Contains(configText, "target_label: __metrics_path__") {
		t.Fatalf("Prometheus config does not use dynamic file discovery/path relabeling:\n%s", configText)
	}
}

func TestBindWritesAttributedTargetAndPrunesOnlySameApplication(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	if _, err := EnsureProviderFiles(); err != nil {
		t.Fatal(err)
	}

	alpha := application.New("alpha", "dev", false, false, false)
	alpha.Services.Postgres = false
	alpha = application.WithWorkload(alpha, "compose.yaml", "api")
	alpha = application.WithMetricsSource(alpha, "application", "api", 8080, "/metrics")

	beta := alpha
	beta.Name = "beta"

	bind := func(m application.Manifest) {
		t.Helper()
		driver := NewDriver(nil, m)
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

	files, err := ExistingProviderFiles()
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
		labels["baseharbor_metrics_path"] != "/metrics" {
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
