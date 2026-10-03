package runtime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func prepareOpenBaoStorage(stateDir, envPath string) error {
	user, err := ensureOpenBaoStorageUser(envPath)
	if err != nil {
		return fmt.Errorf("prepare OpenBao storage user: %w", err)
	}
	secret, err := ensureOpenBaoStorageCredential(envPath)
	if err != nil {
		return fmt.Errorf("prepare OpenBao storage state: %w", err)
	}
	if _, err := ensurePostgresInternalUser(envPath); err != nil {
		return fmt.Errorf("prepare PostgreSQL internal user: %w", err)
	}
	if _, err := ensurePostgresInternalCredential(envPath); err != nil {
		return fmt.Errorf("prepare PostgreSQL internal credential: %w", err)
	}
	if _, err := ensurePostgresReplicationUser(envPath); err != nil {
		return fmt.Errorf("prepare PostgreSQL replication user: %w", err)
	}
	if _, err := ensurePostgresReplicationCredential(envPath); err != nil {
		return fmt.Errorf("prepare PostgreSQL replication credential: %w", err)
	}
	if err := ensureBootstrapPostgresTLS(stateDir); err != nil {
		return fmt.Errorf("prepare PostgreSQL bootstrap trust: %w", err)
	}
	if err := ensureBootstrapOpenBaoTLS(stateDir); err != nil {
		return fmt.Errorf("prepare OpenBao bootstrap trust: %w", err)
	}
	if err := writeOpenBaoPostgresInit(stateDir); err != nil {
		return err
	}
	if err := writeOpenBaoRuntimeConfig(stateDir, user, secret); err != nil {
		return err
	}
	return writeOpenBaoHAProxyConfig(stateDir)
}

func writeOpenBaoPostgresInit(stateDir string) error {
	dir := filepath.Join(stateDir, "providers", "postgresql", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	const script = `#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  --set=baseharbor_user="$BASEHARBOR_POSTGRES_USER" \
  --set=baseharbor_secret="$BASEHARBOR_POSTGRES_PASSWORD" \
  --set=baseharbor_db="$BASEHARBOR_POSTGRES_DB" \
  --set=openbao_user="$BASEHARBOR_OPENBAO_DB_USER" \
  --set=openbao_secret="$BASEHARBOR_OPENBAO_DB_PASSWORD" <<'EOSQL'
SELECT format('CREATE ROLE %I LOGIN SUPERUSER PASSWORD %L', :'baseharbor_user', :'baseharbor_secret')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'baseharbor_user')
\gexec
SELECT format('ALTER ROLE %I WITH LOGIN SUPERUSER PASSWORD %L', :'baseharbor_user', :'baseharbor_secret')
\gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'baseharbor_db', :'baseharbor_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'baseharbor_db')
\gexec
SELECT format('ALTER DATABASE %I OWNER TO %I', :'baseharbor_db', :'baseharbor_user')
\gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', :'baseharbor_db')
\gexec

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'openbao_user', :'openbao_secret')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'openbao_user')
\gexec
SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'openbao_user', :'openbao_secret')
\gexec
SELECT format('CREATE DATABASE openbao OWNER %I', :'openbao_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'openbao')
\gexec
SELECT format('ALTER DATABASE openbao OWNER TO %I', :'openbao_user')
\gexec
REVOKE ALL ON DATABASE openbao FROM PUBLIC;
SELECT format('GRANT CONNECT ON DATABASE openbao TO %I', :'openbao_user')
\gexec
EOSQL

PGPASSWORD="$BASEHARBOR_POSTGRES_PASSWORD" psql -v ON_ERROR_STOP=1 \
  --username "$BASEHARBOR_POSTGRES_USER" --dbname "$BASEHARBOR_POSTGRES_DB" \
  -Atqc 'SELECT 1' >/dev/null

PGPASSWORD="$BASEHARBOR_OPENBAO_DB_PASSWORD" psql -v ON_ERROR_STOP=1 \
  --username "$BASEHARBOR_OPENBAO_DB_USER" --dbname openbao \
  -Atqc 'SELECT 1' >/dev/null
`
	return os.WriteFile(filepath.Join(dir, "openbao-init.sh"), []byte(script), 0o644)
}

func writeOpenBaoHAProxyConfig(stateDir string) error {
	dir := filepath.Join(stateDir, "providers", "openbao", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	const config = `global
  log stdout format raw local0

defaults
  mode tcp
  log global
  timeout connect 5s
  timeout client 60s
  timeout server 60s

frontend openbao
  bind :8200
  default_backend members

backend members
  option httpchk
  http-check send meth GET uri /v1/sys/health ver HTTP/1.1 hdr Host openbao
  http-check expect status 200
  default-server check check-ssl verify none inter 2s fall 2 rise 2 init-addr last,libc,none
  server openbao-1 openbao-member-1:8200
  server openbao-2 openbao-member-2:8200
  server openbao-3 openbao-member-3:8200
`
	return os.WriteFile(filepath.Join(dir, "haproxy.cfg"), []byte(config), 0o644)
}

func writeOpenBaoRuntimeConfig(stateDir, user, secret string) error {
	dir := filepath.Join(stateDir, "providers", "openbao", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	connectionURL := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, secret),
		Host:   "postgres:5432",
		Path:   "/openbao",
		RawQuery: "sslmode=verify-full&sslrootcert=" +
			url.QueryEscape("/run/baseharbor/postgres-ca/ca.pem"),
	}).String()
	config := fmt.Sprintf(`ui = true
disable_mlock = true

storage "postgresql" {
  connection_url      = "%s"
  ha_enabled          = "true"
  max_connect_retries = 0
  max_parallel        = "20"
}

listener "tcp" {
  address                  = "0.0.0.0:8200"
  cluster_address          = "0.0.0.0:8201"
  tls_disable              = false
  tls_cert_file            = "/run/baseharbor/openbao/server-cert.pem"
  tls_key_file             = "/run/baseharbor/openbao/server-key.pem"
  tls_auto_reload          = true
  tls_auto_reload_interval = "10s"
  tls_min_version          = "tls12"
  tls_key_exchange_preferences = ["X25519MLKEM768", "X25519", "CurveP256", "CurveP384"]
}

api_addr = "https://openbao:8200"
`, connectionURL)
	return os.WriteFile(filepath.Join(dir, "openbao.hcl"), []byte(config), 0o644)
}
