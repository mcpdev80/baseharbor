package traces

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestTempoConfigUsesExplicitOTLPAndLocalStorage(t *testing.T) {
	config := configYAML()
	for _, want := range []string{
		"endpoint: 0.0.0.0:4318",
		"backend: local",
		"path: /var/tempo/wal",
		"path: /var/tempo/traces",
		"reporting_enabled: false",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("Tempo config missing %q:\n%s", want, config)
		}
	}
}

func TestTempoComposeIsHardenedAndLoopbackPublished(t *testing.T) {
	rendered := composeYAML(Placement{Scope: capability.ScopeShared, Network: "baseharbor-traces", Volume: "baseharbor-tempo-data"})
	for _, want := range []string{
		"grafana/tempo:3.0.2",
		"user: \"10001:10001\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"127.0.0.1:${BASEHARBOR_TEMPO_PORT}:3200",
		"name: \"baseharbor-traces\"",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Tempo Compose missing %q:\n%s", want, rendered)
		}
	}
}
