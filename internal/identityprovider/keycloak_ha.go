package identityprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	const cfg = `global
  log stdout format raw local0

defaults
  mode tcp
  log global
  timeout connect 5s
  timeout client 30s
  timeout server 30s

frontend postgres
  bind :5432
  default_backend primary

backend primary
  option httpchk GET /primary
  http-check expect status 200
  default-server check port 8008 inter 2s fall 2 rise 2
  server postgres-1 keycloak-db-member-1:5432 check
  server postgres-2 keycloak-db-member-2:5432 check
  server postgres-3 keycloak-db-member-3:5432 check
`
	return os.WriteFile(filepath.Join(root, "haproxy.cfg"), []byte(cfg), 0o644)
}

func keycloakHADataLayerCompose() string {
	const etcdCluster = "keycloak-db-etcd-1=http://keycloak-db-etcd-1:2380,keycloak-db-etcd-2=http://keycloak-db-etcd-2:2380,keycloak-db-etcd-3=http://keycloak-db-etcd-3:2380"
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
		b.WriteString("      - --listen-client-urls=http://0.0.0.0:2379\n")
		fmt.Fprintf(&b, "      - --advertise-client-urls=http://%s:2379\n", name)
		b.WriteString("      - --listen-peer-urls=http://0.0.0.0:2380\n")
		fmt.Fprintf(&b, "      - --initial-advertise-peer-urls=http://%s:2380\n", name)
		fmt.Fprintf(&b, "      - --initial-cluster=%s\n", etcdCluster)
		b.WriteString("      - --initial-cluster-token=baseharbor-keycloak-db\n")
		b.WriteString("      - --initial-cluster-state=new\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - keycloak-db-etcd-data-%d:/etcd-data\n", ordinal)
		b.WriteString("    networks:\n      identity-internal: {}\n\n")
	}

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
		b.WriteString("    environment:\n")
		b.WriteString("      SPILO_PROVIDER: local\n")
		b.WriteString("      SCOPE: baseharbor-keycloak-db\n")
		b.WriteString("      PGVERSION: \"18\"\n")
		fmt.Fprintf(&b, "      ETCD3_HOSTS: %s\n", strconv.Quote("'"+strings.ReplaceAll(etcdHosts, ",", "','")+"'"))
		b.WriteString("      PGUSER_SUPERUSER: postgres\n")
		b.WriteString("      PGPASSWORD_SUPERUSER: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}\n")
		b.WriteString("      PGUSER_STANDBY: keycloak_replication\n")
		b.WriteString("      PGPASSWORD_STANDBY: ${BASEHARBOR_KEYCLOAK_DB_REPLICATION_PASSWORD}\n")

		b.WriteString("      SSL_CERTIFICATE_FILE: /run/baseharbor/db-tls/server.pem\n")
		b.WriteString("      SSL_PRIVATE_KEY_FILE: /run/baseharbor/db-tls/server-key.pem\n")
		b.WriteString("      SSL_TEST_RELOAD: \"true\"\n")
		fmt.Fprintf(&b, "      SPILO_CONFIGURATION: |\n        postgresql:\n          connect_address: %s:5432\n        restapi:\n          connect_address: %s:8008\n", name, name)
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - keycloak-db-data-%d:/home/postgres/pgroot\n", ordinal)
		b.WriteString("      - keycloak-db-tls:/run/baseharbor/db-tls:ro\n")
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
	b.WriteString("        attempts=0; until pg_isready -h keycloak-db -p 5432 -U postgres -d postgres; do attempts=$$((attempts+1)); if [ \"$$attempts\" -ge 90 ]; then echo 'Keycloak PostgreSQL HA endpoint did not become ready' >&2; exit 1; fi; sleep 1; done\n")
	b.WriteString("        role_exists=$$(psql -h keycloak-db -U postgres -d postgres -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -tAc \"SELECT 1 FROM pg_roles WHERE rolname = :'app_user'\")\n")
	b.WriteString("        if [ \"$$role_exists\" != \"1\" ]; then psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -v app_password=\"$$BASEHARBOR_KEYCLOAK_DB_PASSWORD\" -Atc \"SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password')\" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; else psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\" -v app_password=\"$$BASEHARBOR_KEYCLOAK_DB_PASSWORD\" -Atc \"SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password')\" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; fi\n")
	b.WriteString("        exists=$$(psql -h keycloak-db -U postgres -d postgres -v db_name=\"$$BASEHARBOR_KEYCLOAK_DB_NAME\" -tAc \"SELECT 1 FROM pg_database WHERE datname = :'db_name'\")\n")
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
