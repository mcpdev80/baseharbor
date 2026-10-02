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
		"BASEHARBOR_LOKI_COMPACTOR_MODE: main",
		"BASEHARBOR_LOKI_COMPACTOR_MODE: worker",
		"BASEHARBOR_LOKI_COMPACTOR_WORKERS: \"4\"",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("Loki HA compose missing %q:\n%s", want, compose)
		}
	}
}

func TestLokiHACompactorTopologyHasOneMainAndTwoWorkers(t *testing.T) {
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
	if got := strings.Count(compose, "BASEHARBOR_LOKI_COMPACTOR_MODE: main"); got != 1 {
		t.Fatalf("main compactor count = %d, want 1\n%s", got, compose)
	}
	if got := strings.Count(compose, "BASEHARBOR_LOKI_COMPACTOR_MODE: worker"); got != 2 {
		t.Fatalf("worker compactor count = %d, want 2\n%s", got, compose)
	}
	if got := strings.Count(compose, "BASEHARBOR_LOKI_COMPACTOR_WORKERS: \"4\""); got != 3 {
		t.Fatalf("compactor worker capacity count = %d, want 3\n%s", got, compose)
	}
	if strings.Contains(compose, "-compactor.horizontal-scaling-mode=") ||
		strings.Contains(compose, "-compactor.worker.num-sub-workers=") {
		t.Fatalf("Loki HA compose must configure compactor HA through expanded config, not CLI overrides:\n%s", compose)
	}
}

func TestLokiHAConfigEnablesHorizontalCompactorWorkerBackend(t *testing.T) {
	cfg := lokiHAConfig()
	for _, want := range []string{
		"retention_enabled: true",
		"delete_request_store: s3",
		"horizontal_scaling_mode: ${BASEHARBOR_LOKI_COMPACTOR_MODE}",
		"num_sub_workers: ${BASEHARBOR_LOKI_COMPACTOR_WORKERS}",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("Loki HA config missing horizontal compactor worker prerequisite %q:\n%s", want, cfg)
		}
	}
	if strings.Contains(cfg, "retention_period:") {
		t.Fatalf("Loki HA config must not invent a data-retention period:\n%s", cfg)
	}
}
