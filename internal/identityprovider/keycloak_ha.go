package identityprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/dcspki"
)

const (
	keycloakPostgresImage = "ghcr.io/zalando/spilo-18:4.1-p2"
	keycloakEtcdImage     = "gcr.io/etcd-development/etcd:v3.7.2"
	keycloakHAProxyImage  = "docker.io/library/haproxy:3.2.23-alpine"
)

func ensureKeycloakPostgresHA(dir string) error {
	root := filepath.Join(dir, "db-ha")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if _, err := dcspki.Ensure(filepath.Join(root, "etcd-pki"), []string{"keycloak-db-etcd-1", "keycloak-db-etcd-2", "keycloak-db-etcd-3"}); err != nil {
		return fmt.Errorf("prepare Keycloak PostgreSQL DCS mTLS: %w", err)
	}
	const cfg = `global
  log stdout format raw local0

defaults
  mode tcp
  log global
  timeout connect 5s
  timeout client 30s
  timeout server 30s

resolvers container_dns
  parse-resolv-conf
  resolve_retries 3
  timeout retry 1s
  hold valid 5s

frontend postgres
  bind :5432
  default_backend primary

backend primary
  option httpchk GET /primary
  http-check expect status 200
  default-server check port 8008 inter 2s fall 2 rise 2 resolvers container_dns resolve-prefer ipv4 init-addr last,none
  server postgres-1 keycloak-db-member-1:5432 check
  server postgres-2 keycloak-db-member-2:5432 check
  server postgres-3 keycloak-db-member-3:5432 check
`
	return os.WriteFile(filepath.Join(root, "haproxy.cfg"), []byte(cfg), 0o644)
}

func keycloakHADataLayerCompose() string {
	const etcdCluster = "keycloak-db-etcd-1=https://keycloak-db-etcd-1:2380,keycloak-db-etcd-2=https://keycloak-db-etcd-2:2380,keycloak-db-etcd-3=https://keycloak-db-etcd-3:2380"
	const etcdHosts = "keycloak-db-etcd-1:2379,keycloak-db-etcd-2:2379,keycloak-db-etcd-3:2379"

	var b strings.Builder
	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := fmt.Sprintf("keycloak-db-etcd-%d", ordinal)
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", keycloakEtcdImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    command:\n")
		b.WriteString("      - /usr/local/bin/etcd\n")
		fmt.Fprintf(&b, "      - --name=%s\n", name)
		b.WriteString("      - --data-dir=/etcd-data\n")
		b.WriteString("      - --listen-client-urls=https://0.0.0.0:2379\n")
		fmt.Fprintf(&b, "      - --advertise-client-urls=https://%s:2379\n", name)
		b.WriteString("      - --listen-peer-urls=https://0.0.0.0:2380\n")
		fmt.Fprintf(&b, "      - --initial-advertise-peer-urls=https://%s:2380\n", name)
		fmt.Fprintf(&b, "      - --initial-cluster=%s\n", etcdCluster)
		b.WriteString("      - --initial-cluster-token=baseharbor-keycloak-db\n")
		b.WriteString("      - --initial-cluster-state=new\n")
		b.WriteString("      - --cert-file=/run/baseharbor/etcd/server.pem\n")
		b.WriteString("      - --key-file=/run/baseharbor/etcd/server-key.pem\n")
		b.WriteString("      - --client-cert-auth=true\n")
		b.WriteString("      - --trusted-ca-file=/run/baseharbor/etcd/ca.pem\n")
		b.WriteString("      - --peer-cert-file=/run/baseharbor/etcd/server.pem\n")
		b.WriteString("      - --peer-key-file=/run/baseharbor/etcd/server-key.pem\n")
		b.WriteString("      - --peer-client-cert-auth=true\n")
		b.WriteString("      - --peer-trusted-ca-file=/run/baseharbor/etcd/ca.pem\n")
		b.WriteString("      - --tls-min-version=TLS1.2\n")
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"127.0.0.1:${BASEHARBOR_KEYCLOAK_ETCD_PORT_%d}:2379\"\n", ordinal)
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - keycloak-db-etcd-data-%d:/etcd-data\n", ordinal)
		b.WriteString("      - ./db-ha/etcd-pki:/run/baseharbor/etcd:ro\n")
		b.WriteString("    networks:\n      identity-internal: {}\n\n")
	}

	b.WriteString("  keycloak-db-etcd-recovery:\n")
	fmt.Fprintf(&b, "    image: %s\n", keycloakEtcdImage)
	b.WriteString("    profiles: [\"recovery\"]\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    entrypoint: [\"/usr/local/bin/etcdctl\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./db-ha/etcd-pki:/run/baseharbor/etcd:ro\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev,mode=0700\n")
	b.WriteString("    networks:\n      identity-internal: {}\n\n")

	b.WriteString("  keycloak-db-tls-init:\n")
	fmt.Fprintf(&b, "    image: %s\n", keycloakPostgresImage)
	b.WriteString("    restart: \"no\"\n")
	b.WriteString("    user: \"0:0\"\n")
	b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
	b.WriteString("    command:\n")
	b.WriteString("      - |\n")
	b.WriteString("        uid=$$(id -u postgres); gid=$$(id -g postgres)\n")
	b.WriteString("        cp /source/ca.pem /target/ca.pem\n")
	b.WriteString("        cp /source/server.pem /target/server.pem\n")
	b.WriteString("        cp /source/server-key.pem /target/server-key.pem\n")
	b.WriteString("        chown \"$$uid:$$gid\" /target/ca.pem /target/server.pem /target/server-key.pem\n")
	b.WriteString("        chmod 0644 /target/ca.pem /target/server.pem\n")
	b.WriteString("        chmod 0600 /target/server-key.pem\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./db-ha/runtime:/source:ro\n")
	b.WriteString("      - keycloak-db-tls:/target\n")
	b.WriteString("    networks:\n      identity-internal: {}\n\n")

	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := fmt.Sprintf("keycloak-db-member-%d", ordinal)
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", keycloakPostgresImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    depends_on:\n")
		b.WriteString("      keycloak-db-tls-init:\n")
		b.WriteString("        condition: service_completed_successfully\n")
		b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
		b.WriteString("    command:\n")
		b.WriteString("      - |\n")
		b.WriteString("        install -d -o postgres -g postgres -m 0700 /run/baseharbor/etcd-runtime\n")
		b.WriteString("        install -o postgres -g postgres -m 0644 /run/baseharbor/etcd-source/ca.pem /run/baseharbor/etcd-runtime/ca.pem\n")
		b.WriteString("        install -o postgres -g postgres -m 0644 /run/baseharbor/etcd-source/client.pem /run/baseharbor/etcd-runtime/client.pem\n")
		b.WriteString("        install -o postgres -g postgres -m 0600 /run/baseharbor/etcd-source/client-key.pem /run/baseharbor/etcd-runtime/client-key.pem\n")
		b.WriteString("        exec /bin/sh /launch.sh\n")
		b.WriteString("    environment:\n")
		b.WriteString("      SPILO_PROVIDER: local\n")
		b.WriteString("      SCOPE: baseharbor-keycloak-db\n")
		b.WriteString("      PGVERSION: \"18\"\n")
		b.WriteString("      PGROOT: /home/postgres/pgdata/pgroot\n")
		b.WriteString("      PGDATA: /home/postgres/pgdata/pgroot/data\n")
		fmt.Fprintf(&b, "      ETCD3_HOSTS: %s\n", strconv.Quote("'"+strings.ReplaceAll(etcdHosts, ",", "','")+"'"))
		// Spilo builds Patroni's configuration from ETCD3_* before launching it.
		// Set both forms so that generation cannot replace mutual TLS with HTTP.
		b.WriteString("      ETCD3_PROTOCOL: https\n")
		b.WriteString("      ETCD3_CACERT: /run/baseharbor/etcd-runtime/ca.pem\n")
		b.WriteString("      ETCD3_CERT: /run/baseharbor/etcd-runtime/client.pem\n")
		b.WriteString("      ETCD3_KEY: /run/baseharbor/etcd-runtime/client-key.pem\n")
		b.WriteString("      PATRONI_ETCD3_HOSTS: " + etcdHosts + "\n")
		b.WriteString("      PATRONI_ETCD3_PROTOCOL: https\n")
		b.WriteString("      PATRONI_ETCD3_CACERT: /run/baseharbor/etcd-runtime/ca.pem\n")
		b.WriteString("      PATRONI_ETCD3_CERT: /run/baseharbor/etcd-runtime/client.pem\n")
		b.WriteString("      PATRONI_ETCD3_KEY: /run/baseharbor/etcd-runtime/client-key.pem\n")
		b.WriteString("      PGUSER_SUPERUSER: postgres\n")
		b.WriteString("      PGPASSWORD_SUPERUSER: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}\n")
		b.WriteString("      PGUSER_STANDBY: keycloak_replication\n")
		b.WriteString("      PGPASSWORD_STANDBY: ${BASEHARBOR_KEYCLOAK_DB_REPLICATION_PASSWORD}\n")

		b.WriteString("      SSL_CERTIFICATE_FILE: /run/baseharbor/db-tls/server.pem\n")
		b.WriteString("      SSL_PRIVATE_KEY_FILE: /run/baseharbor/db-tls/server-key.pem\n")
		b.WriteString("      SSL_TEST_RELOAD: \"true\"\n")
		fmt.Fprintf(&b, "      SPILO_CONFIGURATION: %s\n", strconv.Quote(fmt.Sprintf(`{"name":"%s","postgresql":{"connect_address":"%s:5432"},"restapi":{"connect_address":"%s:8008"}}`, name, name, name)))
		b.WriteString("    tmpfs:\n")
		b.WriteString("      - /run/baseharbor/etcd-runtime:rw,noexec,nosuid,nodev,mode=0700\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - keycloak-db-data-%d:/home/postgres/pgdata/pgroot\n", ordinal)
		b.WriteString("      - keycloak-db-tls:/run/baseharbor/db-tls:ro\n")
		b.WriteString("      - ./db-ha/etcd-pki:/run/baseharbor/etcd-source:ro\n")
		b.WriteString("    networks:\n      identity-internal: {}\n\n")
	}

	b.WriteString("  keycloak-db:\n")
	fmt.Fprintf(&b, "    image: %s\n", keycloakHAProxyImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"99:99\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./db-ha/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro\n")
	b.WriteString("    networks:\n      identity-internal: {}\n\n")

	b.WriteString("  keycloak-db-init:\n")
	b.WriteString("    image: docker.io/library/postgres:18-alpine\n")
	b.WriteString("    restart: \"no\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    depends_on:\n")
	b.WriteString("      keycloak-db-tls-init:\n")
	b.WriteString("        condition: service_completed_successfully\n")
	b.WriteString("    environment:\n")
	b.WriteString("      PGPASSWORD: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}\n")
	b.WriteString("      BASEHARBOR_KEYCLOAK_DB_USER: ${BASEHARBOR_KEYCLOAK_DB_USER}\n")
	b.WriteString("      BASEHARBOR_KEYCLOAK_DB_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}\n")
	b.WriteString("      BASEHARBOR_KEYCLOAK_DB_NAME: ${BASEHARBOR_KEYCLOAK_DB_NAME}\n")
	b.WriteString("      PGSSLMODE: verify-full\n")
	b.WriteString("      PGSSLROOTCERT: /run/baseharbor/db-tls/ca.pem\n")
	b.WriteString("      PGCONNECT_TIMEOUT: \"1\"\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - keycloak-db-tls:/run/baseharbor/db-tls:ro\n")
	b.WriteString("    command:\n")
	b.WriteString("      - /bin/sh\n")
	b.WriteString("      - -ec\n")
	b.WriteString("      - |\n")
	b.WriteString("        attempts=0; until psql -h keycloak-db -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null; do attempts=$$((attempts+1)); if [ \"$$attempts\" -ge 90 ]; then echo 'Keycloak PostgreSQL HA endpoint did not become ready' >&2; exit 1; fi; sleep 1; done\n")
	b.WriteString("        role_exists=$$(printf '%s\\n' \"SELECT 1 FROM pg_roles WHERE rolname = :'app_user';\" | psql -h keycloak-db -U postgres -d postgres -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -tA)\n")
	b.WriteString("        if [ \"$$role_exists\" != \"1\" ]; then printf '%s\\n' \"SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password');\" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -v app_password=\"$$BASEHARBOR_KEYCLOAK_DB_PASSWORD\" -At | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; else printf '%s\\n' \"SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password');\" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -v app_password=\"$$BASEHARBOR_KEYCLOAK_DB_PASSWORD\" -At | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; fi\n")
	b.WriteString("        exists=$$(printf '%s\\n' \"SELECT 1 FROM pg_database WHERE datname = :'db_name';\" | psql -h keycloak-db -U postgres -d postgres -v db_name=\"$$BASEHARBOR_KEYCLOAK_DB_NAME\" -tA)\n")
	b.WriteString("        if [ \"$$exists\" != \"1\" ]; then createdb -h keycloak-db -U postgres -O \"$$BASEHARBOR_KEYCLOAK_DB_USER\" \"$$BASEHARBOR_KEYCLOAK_DB_NAME\"; fi\n")
	b.WriteString("    networks:\n      identity-internal: {}\n\n")
	return b.String()
}

func keycloakHAVolumesCompose() string {
	var b strings.Builder
	b.WriteString("  keycloak-db-tls:\n")
	for ordinal := 1; ordinal <= 3; ordinal++ {
		fmt.Fprintf(&b, "  keycloak-db-data-%d:\n", ordinal)
		fmt.Fprintf(&b, "  keycloak-db-etcd-data-%d:\n", ordinal)
	}
	return b.String()
}
