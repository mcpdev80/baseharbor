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
		`until rpk cluster health -X admin.hosts="$admin_hosts" --exit-when-healthy`,
		`attempts=$$((attempts+1))`,
		`if [ "$$attempts" -ge 45 ]`,
		`rpk topic create tempo-traces -X brokers="$$brokers"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Tempo HA Compose missing escaped init expression %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{
		`-X brokers="$brokers"`,
		`-X admin.hosts="$admin_hosts"`,
		`rpk cluster health -X brokers=`,
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Tempo HA Compose contains invalid or unescaped init expression %q:\n%s", forbidden, rendered)
		}
	}
}

func TestTempoHAQueryGatewayUsesStableAlias(t *testing.T) {
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
	if got := strings.Count(rendered, "- tempo-query-frontend"); got < 2 {
		t.Fatalf("both Tempo query frontends must publish the shared alias, got %d:\n%s", got, rendered)
	}
	if !strings.Contains(rendered, "reverse_proxy tempo-query-frontend:3200") {
		t.Fatalf("Tempo HA gateway must route through the stable query-frontend alias:\n%s", rendered)
	}
	if strings.Contains(rendered, "reverse_proxy tempo-query-frontend-1:3200 tempo-query-frontend-2:3200") {
		t.Fatalf("Tempo HA gateway must not depend on stopped-container DNS names:\n%s", rendered)
	}
}
