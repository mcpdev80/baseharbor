package metrics

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProviderComposeAttachesOnlyDeclaredProviderNetworks(t *testing.T) {
	rendered := providerComposeYAMLWithProviderNetworks(
		Placement{Scope: capability.ScopeShared, Project: "baseharbor-metrics", Volume: "baseharbor-prometheus-data"},
		nil,
		[]string{"baseharbor-logs-internal", "baseharbor-telemetry"},
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
	first := providerTargetFileName("tempo:baseharbor-traces")
	second := providerTargetFileName("tempo:baseharbor-traces")
	if first != second || !strings.HasPrefix(first, "provider--") || !strings.HasSuffix(first, ".json") {
		t.Fatalf("unexpected provider target filename %q / %q", first, second)
	}
}
