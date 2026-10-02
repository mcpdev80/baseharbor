package traces

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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
		"docker.io/grafana/tempo:3.0.2",
		"user: \"10001:10001\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"127.0.0.1:${BASEHARBOR_TEMPO_PORT}:8443",
		"name: \"baseharbor-traces\"",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Tempo Compose missing %q:\n%s", want, rendered)
		}
	}
}

func TestTempoHAComposePreservesKafkaInitShellVariables(t *testing.T) {
	rendered := tempoHACompose(
		Placement{Scope: capability.ScopeShared, Network: "baseharbor-traces", Volume: "baseharbor-tempo-data"},
		serviceaccess.HTTPGatewayFiles{
			Caddyfile: "./service-access/Caddyfile",
			Material: serviceaccess.TLSMaterial{
				CA:                "./service-access/runtime/ca.pem",
				ServerCertificate: "./service-access/runtime/server.pem",
				ServerKey:         "./service-access/runtime/server-key.pem",
			},
		},
		objectstorage.PlatformBucket{Network: "baseharbor-object-storage"},
	)
	for _, want := range []string{
		`until rpk cluster health -X brokers="$brokers"`,
		`attempts=$$((attempts+1))`,
		`if [ "$$attempts" -ge 45 ]`,
		`rpk topic create tempo-traces -X brokers="$brokers"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Tempo HA Compose missing escaped init expression %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `-X brokers="$brokers"`) {
		t.Fatalf("Tempo HA Compose contains unescaped Compose variable interpolation:\n%s", rendered)
	}
}
