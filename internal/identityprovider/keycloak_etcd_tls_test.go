package identityprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeycloakHAEtcdUsesMutualTLS(t *testing.T) {
	dir := t.TempDir()
	if err := ensureKeycloakPostgresHA(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ca.pem", "ca-key.pem", "server.pem", "server-key.pem", "client.pem", "client-key.pem"} {
		if _, err := os.Stat(filepath.Join(dir, "db-ha", "etcd-pki", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	compose := keycloakHADataLayerCompose()
	for _, required := range []string{
		"--listen-client-urls=https://0.0.0.0:2379",
		"--client-cert-auth=true",
		"--peer-client-cert-auth=true",
		"keycloak-db-etcd-1=https://keycloak-db-etcd-1:2380",
		"PATRONI_ETCD3_PROTOCOL: https",
		"/run/baseharbor/etcd-runtime/client-key.pem",
		"127.0.0.1:${BASEHARBOR_KEYCLOAK_ETCD_PORT_1}:2379",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("HA compose missing %q", required)
		}
	}
	if strings.Contains(compose, "--listen-client-urls=http://") || strings.Contains(compose, "=http://keycloak-db-etcd-") {
		t.Fatal("plaintext etcd transport remains in Keycloak HA compose")
	}
}
