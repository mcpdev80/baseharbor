package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestSyncPrometheusAlertRulesRequiresEffectiveTargetAndPrunesOwnedRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, baseHarborAlertRuleFile)
	source := observability.MetricsSource{
		ID: "provider:runtime", Provider: capability.ProviderPrometheus,
		Class: observability.SourcePlatformProvider, Target: "prometheus:9464",
		Path: "/metrics",
	}
	if err := syncPrometheusAlertRules(dir, []observability.MetricsSource{source}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BaseHarborTargetUnavailable", `up{job=~"baseharbor-applications|baseharbor-providers"} == 0`, "severity: warning"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Prometheus rule is missing %q:\n%s", want, data)
		}
	}
	if err := syncPrometheusAlertRules(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("managed rule remains without effective targets: stat err = %v", err)
	}
}

func TestSyncPrometheusAlertRulesSkipsUnverifiedTargets(t *testing.T) {
	dir := t.TempDir()
	source := observability.MetricsSource{
		ID: "provider:runtime", Provider: capability.ProviderPrometheus,
		Class: observability.SourcePlatformProvider, Target: "prometheus:9464",
		Verification: capability.ObservabilityVerifyNone,
	}
	if err := syncPrometheusAlertRules(dir, []observability.MetricsSource{source}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, baseHarborAlertRuleFile)); !os.IsNotExist(err) {
		t.Fatalf("unverified target enabled alert rule: stat err = %v", err)
	}
}

func TestPrometheusConfigLoadsRulesAndEvaluatesAtScrapeInterval(t *testing.T) {
	config := prometheusConfig(nil, false)
	for _, want := range []string{"evaluation_interval: 5s", "/etc/prometheus/rules/*.yml"} {
		if !strings.Contains(config, want) {
			t.Fatalf("Prometheus config missing %q:\n%s", want, config)
		}
	}
	compose := providerComposeYAMLWithProviderNetworksAndAccess(
		Placement{Scope: capability.ScopeShared, Project: "metrics", Volume: "metrics-data"},
		nil, nil, false, false,
		serviceaccess.HTTPGatewayFiles{},
	)
	if !strings.Contains(compose, "./rules:/etc/prometheus/rules:ro") {
		t.Fatalf("Prometheus does not mount its managed rules read-only:\n%s", compose)
	}
}
