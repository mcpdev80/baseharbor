package runtime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func prepareOpenBaoStorage(stateDir, envPath string) error {
	secret, err := ensureOpenBaoStorageCredential(envPath)
	if err != nil {
		return fmt.Errorf("prepare OpenBao storage state: %w", err)
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
	if err := writeOpenBaoRuntimeConfig(stateDir, secret); err != nil {
		return err
	}
	return nil
}

func writeOpenBaoPostgresInit(stateDir string) error {
	dir := filepath.Join(stateDir, "providers", "postgresql", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	const script = `#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=openbao_secret="$BASEHARBOR_OPENBAO_DB_PASSWORD" <<'EOSQL'
SELECT format('CREATE ROLE openbao LOGIN PASSWORD %L', :'openbao_secret')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'openbao')
\gexec
SELECT 'CREATE DATABASE openbao OWNER openbao'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'openbao')
\gexec
REVOKE ALL ON DATABASE openbao FROM PUBLIC;
GRANT CONNECT ON DATABASE openbao TO openbao;
EOSQL
`
	return os.WriteFile(filepath.Join(dir, "openbao-init.sh"), []byte(script), 0o700)
}

func writeOpenBaoRuntimeConfig(stateDir, secret string) error {
	dir := filepath.Join(stateDir, "providers", "openbao", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	escaped := url.QueryEscape(secret)
	config := fmt.Sprintf(`ui = true
disable_mlock = true

storage "postgresql" {
  connection_url = "postgres://openbao:%s@postgres:5432/openbao?sslmode=verify-full&sslrootcert=/run/baseharbor/postgres-ca/ca.pem"
}

listener "tcp" {
  address                  = "0.0.0.0:8200"
  tls_disable              = false
  tls_cert_file            = "/run/baseharbor/tls-source/server-cert.pem"
  tls_key_file             = "/run/baseharbor/tls-source/server-key.pem"
  tls_auto_reload          = true
  tls_auto_reload_interval = "10s"
  tls_min_version          = "tls12"
  tls_key_exchange_preferences = ["X25519MLKEM768", "X25519", "CurveP256", "CurveP384"]
}

api_addr = "https://openbao:8200"
`, escaped)
	return os.WriteFile(filepath.Join(dir, "openbao.hcl"), []byte(config), 0o600)
}
