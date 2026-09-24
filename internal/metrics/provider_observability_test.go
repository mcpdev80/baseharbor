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

func TestProviderComposeAttachesOnlyDeclaredProviderNetworks(t *testing.T) {
	rendered := providerComposeYAMLWithProviderNetworks(
		Placement{Scope: capability.ScopeShared, Project: "baseharbor-metrics", Volume: "baseharbor-prometheus-data"},
		nil,
		[]string{"baseharbor-logs-internal", "baseharbor-telemetry"},
		false,
	)
	for _, want := range []string{
		"provider-0:",
		"provider-1:",
		"external: true",
		"name: \"baseharbor-logs-internal\"",
		"name: \"baseharbor-telemetry\"",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Prometheus provider Compose missing %q:\n%s", want, rendered)
		}
	}
}

func TestProviderTargetFileNameIsStableAndNamespaced(t *testing.T) {
	source := observability.MetricsSource{ID: "tempo:baseharbor-traces"}
	first := providerTargetFileName(source)
	second := providerTargetFileName(source)
	if first != second || !strings.HasPrefix(first, "provider--") || !strings.HasSuffix(first, ".json") {
		t.Fatalf("unexpected provider target filename %q / %q", first, second)
	}

	source.Security.TLSRequired = true
	secure := providerTargetFileName(source)
	if !strings.HasPrefix(secure, "provider-secure--") || secure == first {
		t.Fatalf("unexpected secure provider target filename %q", secure)
	}
}

func TestPrometheusConfigSeparatesApplicationAndProviderTargets(t *testing.T) {
	rendered := prometheusConfig(nil, false)
	for _, want := range []string{
		"job_name: baseharbor-applications",
		"/etc/prometheus/targets/*--*--*.json",
		"job_name: baseharbor-providers",
		"/etc/prometheus/targets/provider--*.json",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Prometheus config missing %q:\n%s", want, rendered)
		}
	}
}

func TestSecureProviderMetricsRenderMTLSScrapeContract(t *testing.T) {
	root := t.TempDir()
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	source := observability.MetricsSource{
		ID:               "runtime-broker:demo:dev",
		Provider:         capability.ProviderPostgreSQL,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-demo-dev",
		Target:           "baseharbor-runtime:8443",
		Path:             "/metrics",
		Scheme:           "https",
		Security: observability.Security{
			TLSRequired:       true,
			Authentication:    "mtls",
			TrustFile:         write("ca.pem", "ca"),
			ClientCertificate: write("client.pem", "cert"),
			ClientKey:         write("client-key.pem", "key"),
			ServerName:        "baseharbor-runtime",
		},
	}
	securityDir := filepath.Join(root, "provider-security")
	if err := syncProviderSecurity(securityDir, []observability.MetricsSource{source}); err != nil {
		t.Fatal(err)
	}
	token := providerSourceToken(source.ID)
	for name, want := range map[string]string{
		token + "-ca.pem":         "ca",
		token + "-client.pem":     "cert",
		token + "-client-key.pem": "key",
	} {
		data, err := os.ReadFile(filepath.Join(securityDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}

	rendered := prometheusConfig(nil, false, []observability.MetricsSource{source})
	for _, want := range []string{
		"job_name: baseharbor-provider-secure-" + token,
		"scheme: https",
		"metrics_path: \"/metrics\"",
		"/etc/prometheus/targets/" + providerTargetFileName(source),
		"ca_file: /etc/prometheus/provider-security/" + token + "-ca.pem",
		"cert_file: /etc/prometheus/provider-security/" + token + "-client.pem",
		"key_file: /etc/prometheus/provider-security/" + token + "-client-key.pem",
		"server_name: \"baseharbor-runtime\"",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("secure provider scrape config missing %q:\n%s", want, rendered)
		}
	}
}

func TestProviderComposeMountsSecurityOnlyWhenNeeded(t *testing.T) {
	placement := Placement{Scope: capability.ScopeShared, Project: "baseharbor-metrics", Volume: "baseharbor-prometheus-data"}
	access := serviceaccessFixture()
	secure := providerComposeYAMLWithProviderNetworksAndAccess(placement, nil, nil, false, true, access)
	if !strings.Contains(secure, "./provider-security:/etc/prometheus/provider-security:ro") {
		t.Fatalf("secure provider compose missing security mount:\n%s", secure)
	}
	insecure := providerComposeYAMLWithProviderNetworksAndAccess(placement, nil, nil, false, false, access)
	if strings.Contains(insecure, "./provider-security:/etc/prometheus/provider-security:ro") {
		t.Fatalf("insecure provider compose unexpectedly mounts security material:\n%s", insecure)
	}
}

func serviceaccessFixture() serviceaccess.HTTPGatewayFiles {
	return serviceaccess.HTTPGatewayFiles{
		Caddyfile: "./service-access/Caddyfile",
		Material: serviceaccess.TLSMaterial{
			CA:                "./service-access/runtime/ca.pem",
			ServerCertificate: "./service-access/runtime/server.pem",
			ServerKey:         "./service-access/runtime/server-key.pem",
		},
	}
}
