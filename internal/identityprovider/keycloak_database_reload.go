package identityprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Spilo copies the PostgreSQL private key into its runtime directory at
// startup. Refresh that copy before signaling every member to reload TLS,
// while both CAs are trusted and before retiring the previous CA.
func reloadKeycloakDatabaseCertificates(ctx context.Context, runtime KeycloakRuntime, files KeycloakFiles) error {
	executor, ok := runtime.(keycloakProjectUserExecutor)
	if !ok {
		return errors.New("Keycloak PKI rotation requires runtime database reload as the postgres user")
	}
	reloadCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return err
	}
	services := []string{"keycloak-db"}
	keyPath := keycloakDatabaseTLSKeyPaths{
		certificate: "/run/baseharbor/db-tls/server.pem",
		sourceKey:   "/run/baseharbor/db-tls/server-key.pem",
		runtimeKey:  "/tmp/server-key.pem",
	}
	if values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] == "ha" {
		services = []string{"keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-3"}
		keyPath = keycloakDatabaseTLSKeyPaths{
			certificate: "/run/baseharbor/db-tls-source/server.pem",
			sourceKey:   "/run/baseharbor/db-tls-source/server-key.pem",
			runtimeKey:  "/run/baseharbor/etcd-runtime/server-key.pem",
		}
	} else if values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] != "single" {
		return errors.New("Keycloak PostgreSQL TLS reload requires a known, owned topology")
	}
	reloadCommand := keycloakDatabaseTLSReloadCommand(keyPath)
	for _, service := range services {
		var lastOutput string
		var lastErr error
		for {
			attemptCtx, attemptCancel := context.WithTimeout(reloadCtx, 10*time.Second)
			output, err := executor.ExecProjectAs(attemptCtx, files.Project, files.Compose, files.Env, service, "postgres",
				"sh", "-ec", reloadCommand)
			attemptCancel()
			if err == nil {
				break
			}
			lastOutput = strings.TrimSpace(output)
			lastErr = err
			select {
			case <-reloadCtx.Done():
				return fmt.Errorf("reload Keycloak PostgreSQL certificates on %s: %w (last command output: %q)", service, errors.Join(reloadCtx.Err(), lastErr), lastOutput)
			case <-ticker.C:
			}
		}
	}
	return nil
}

type keycloakDatabaseTLSKeyPaths struct {
	certificate string
	sourceKey   string
	runtimeKey  string
}

func keycloakDatabaseTLSReloadCommand(paths keycloakDatabaseTLSKeyPaths) string {
	return fmt.Sprintf(`if [ ! -s "$PGDATA/postmaster.pid" ]; then echo 'postgres postmaster pid is not ready' >&2; exit 1; fi
pid=$(sed -n '1p' "$PGDATA/postmaster.pid")
case "$pid" in ''|*[!0-9]*) exit 1 ;; esac
[ -r "/proc/$pid/comm" ] || { echo 'postgres process is not ready' >&2; exit 1; }
comm=$(cat "/proc/$pid/comm")
case "$comm" in postgres|postmaster) ;; *) echo "unexpected postgres process: $comm" >&2; exit 1 ;; esac
source_cert=%q
source_key=%q
runtime_key=%q
[ -f "$source_cert" ] && [ -r "$source_cert" ] || { echo 'postgres server certificate is unavailable' >&2; exit 1; }
[ -f "$source_key" ] && [ -r "$source_key" ] || { echo 'postgres server key is unavailable' >&2; exit 1; }
[ ! -L "$runtime_key" ] || { echo 'postgres runtime key must not be a symlink' >&2; exit 1; }
umask 077
tmp_key=$(mktemp "${runtime_key}.next.XXXXXX")
cert_pub="${tmp_key}.cert.pub"
key_pub="${tmp_key}.key.pub"
probe_output="${tmp_key}.probe"
served_cert="${tmp_key}.served.pem"
trap 'rm -f "$tmp_key" "$cert_pub" "$key_pub" "$probe_output" "$served_cert"' EXIT HUP INT TERM
cp "$source_key" "$tmp_key"
chmod 0600 "$tmp_key"
openssl x509 -in "$source_cert" -pubkey -noout > "$cert_pub"
openssl pkey -in "$tmp_key" -pubout > "$key_pub"
cmp -s "$cert_pub" "$key_pub" || { echo 'postgres server certificate and key do not match' >&2; exit 1; }
mv -f "$tmp_key" "$runtime_key"
kill -HUP "$pid"
if ! timeout 5 openssl s_client -starttls postgres -connect 127.0.0.1:5432 -showcerts </dev/null > "$probe_output" 2>&1; then echo 'postgres TLS probe failed after certificate reload' >&2; cat "$probe_output" >&2; exit 1; fi
sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' "$probe_output" > "$served_cert"
[ -s "$served_cert" ] || { echo 'postgres TLS probe returned no server certificate' >&2; exit 1; }
expected_fingerprint=$(openssl x509 -in "$source_cert" -noout -fingerprint -sha256)
served_fingerprint=$(openssl x509 -in "$served_cert" -noout -fingerprint -sha256)
[ "$served_fingerprint" = "$expected_fingerprint" ] || { echo "postgres still serves a stale certificate (expected $expected_fingerprint, got $served_fingerprint)" >&2; exit 1; }`, paths.certificate, paths.sourceKey, paths.runtimeKey)
}
