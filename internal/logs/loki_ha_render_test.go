package logs

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestLokiHARenderUsesProcessTrustStoreForSeaweedFS(t *testing.T) {
	cfg := lokiHAConfig()
	if strings.Contains(cfg, "tls_ca_path") {
		t.Fatalf("Loki HA config contains unsupported aws.http_config tls_ca_path:\n%s", cfg)
	}

	placement := Placement{
		Scope:       capability.ScopeShared,
		Project:     "bh-test-shared",
		Network:     "bh-test-logs",
		LokiVolume:  "bh-test-loki",
		AlloyVolume: "bh-test-alloy",
	}
	access := serviceaccess.HTTPGatewayFiles{
		Caddyfile: "./service-access/Caddyfile",
		Material: serviceaccess.TLSMaterial{
			CA:                "./service-access/runtime/ca.pem",
			ServerCertificate: "./service-access/runtime/server.pem",
			ServerKey:         "./service-access/runtime/server-key.pem",
		},
	}
	compose := providerComposeYAMLForModeAndAccess(
		placement,
		nil,
		bhruntime.LogCollectionSyslog,
		access,
		0,
		"bh-test-object-storage",
	)
	for _, want := range []string{
		"SSL_CERT_FILE: /run/baseharbor/object-storage/ca.pem",
		"./object-storage-ca.pem:/run/baseharbor/object-storage/ca.pem:ro",
		"loki-1:",
		"loki-2:",
		"loki-3:",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("Loki HA compose missing %q:\n%s", want, compose)
		}
	}
}
