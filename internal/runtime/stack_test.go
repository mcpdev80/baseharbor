package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestEnsureFilesCreatesProtectedRuntimeState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{files.Compose, files.Env} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", path, info.Mode().Perm())
		}
	}

	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "BASEHARBOR_POSTGRES_PASSWORD=") {
		t.Fatal("runtime environment does not contain postgres password")
	}
	if strings.Contains(string(env), "BASEHARBOR_POSTGRES_PASSWORD=postgres\n") {
		t.Fatal("runtime environment uses an insecure default password")
	}
}

func TestEnsureFilesPreservesExistingSecret(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureFiles(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing runtime environment was overwritten")
	}
}

func TestEnsureFilesPreparesPostgreSQLBackedOpenBao27(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFilesWithPorts(dir, Ports{Postgres: 15432, OpenBao: 18200})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenBaoDBPassword == "" || cfg.OpenBaoDBPassword == cfg.PostgresPassword {
		t.Fatal("OpenBao storage must use a dedicated PostgreSQL credential")
	}
	config, err := os.ReadFile(filepath.Join(dir, "providers", "openbao", "runtime", "openbao.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, want := range []string{
		`storage "postgresql"`,
		"sslmode=verify-full",
		"tls_auto_reload          = true",
		"X25519MLKEM768",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("OpenBao 2.7 runtime config missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{`storage "file"`, `storage "raft"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("OpenBao runtime contains obsolete storage backend %q", forbidden)
		}
	}
	for _, path := range []string{
		filepath.Join(dir, "providers", "postgresql", "runtime", "openbao-init.sh"),
		filepath.Join(dir, "providers", "postgresql", "runtime", "ca.pem"),
		filepath.Join(dir, "providers", "openbao", "runtime", "ca.pem"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("OpenBao bootstrap prerequisite %s: %v", path, err)
		}
	}
	initScript, err := os.ReadFile(filepath.Join(dir, "providers", "postgresql", "runtime", "openbao-init.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`PGPASSWORD="$BASEHARBOR_POSTGRES_PASSWORD" psql`,
		`--username "$BASEHARBOR_POSTGRES_USER" --dbname "$BASEHARBOR_POSTGRES_DB"`,
		`PGPASSWORD="$BASEHARBOR_OPENBAO_DB_PASSWORD" psql`,
		`--username "$BASEHARBOR_OPENBAO_DB_USER" --dbname openbao`,
	} {
		if !strings.Contains(string(initScript), want) {
			t.Fatalf("OpenBao PostgreSQL init script missing credential verification %q", want)
		}
	}
	for _, path := range []string{
		filepath.Join(dir, "providers", "postgresql", "runtime", "openbao-init.sh"),
		filepath.Join(dir, "providers", "openbao", "runtime", "openbao.hcl"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o044 == 0 {
			t.Fatalf("bind-mounted runtime file %s is not readable by the non-root container", path)
		}
	}
}

func TestEnsureFilesWithPortsWritesSelectedPorts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFilesWithPorts(dir, Ports{Postgres: 15432, OpenBao: 18200})
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	text := string(env)
	if !strings.Contains(text, "BASEHARBOR_POSTGRES_PORT=15432\n") {
		t.Fatal("selected PostgreSQL port was not persisted")
	}
	if !strings.Contains(text, "BASEHARBOR_OPENBAO_PORT=18200\n") {
		t.Fatal("selected OpenBao port was not persisted")
	}
}

func TestEnsureFilesDoesNotMaterializeServiceAccessBeforeIssuerIsReady(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, forbidden := range []string{"openbao-access:", "postgres-access:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("bootstrap runtime unexpectedly contains %s before issuer readiness", forbidden)
		}
	}
}

func TestEnsureServiceAccessMaterializesNativeTLSForPostgresAndOpenBao(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	files, err := EnsureFilesWithPorts(dir, Ports{Postgres: 15432, OpenBao: 18200})
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureServiceAccess(context.Background(), serviceissuer.New(t), files); err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, wanted := range []string{
		"127.0.0.1:${BASEHARBOR_OPENBAO_PORT}:8200",
		"BAO_ADDR: https://127.0.0.1:8200",
		"command: [\"server\", \"-config=/run/baseharbor/openbao/openbao.hcl\"]",
		"./providers/openbao/runtime/server-cert.pem:/run/baseharbor/openbao/server-cert.pem:ro",
		"127.0.0.1:${BASEHARBOR_POSTGRES_PORT}:5432",
		"ghcr.io/zalando/spilo-18:4.1-p2",
		"postgres-member-1",
		"PATRONI_NAME: postgres-member-1",
		"PATRONI_NAME: postgres-member-2",
		"PATRONI_NAME: postgres-member-3",
		"ETCD3_HOSTS: \"'postgres-etcd-1:2379','postgres-etcd-2:2379','postgres-etcd-3:2379'\"",
		"SSL_CERTIFICATE_FILE: /run/baseharbor/tls/server-cert.pem",
		"./providers/postgresql/runtime/server-cert.pem:/run/baseharbor/tls/server-cert.pem:ro",
		"./providers/postgresql/runtime/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("reconciled runtime is missing %q", wanted)
		}
	}
	if strings.Contains(text, "postgres-access:") {
		t.Fatal("control-plane PostgreSQL stable endpoint must remain named postgres")
	}
	hba, err := os.ReadFile(filepath.Join(dir, "providers", "postgresql", "runtime", "pg_hba.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"hostssl all all 0.0.0.0/0 scram-sha-256", "hostnossl all all 0.0.0.0/0 reject"} {
		if !strings.Contains(string(hba), wanted) {
			t.Fatalf("pg_hba.conf missing %q", wanted)
		}
	}
	config, err := os.ReadFile(filepath.Join(dir, "providers", "openbao", "runtime", "openbao.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	configText := string(config)
	for _, wanted := range []string{
		`storage "postgresql"`,
		"sslmode=verify-full",
		"tls_auto_reload          = true",
		"X25519MLKEM768",
	} {
		if !strings.Contains(configText, wanted) {
			t.Fatalf("OpenBao 2.7 runtime config missing %q", wanted)
		}
	}
}

func TestEnsureFilesWithPortsRejectsDuplicatePort(t *testing.T) {
	if _, err := EnsureFilesWithPorts(filepath.Join(t.TempDir(), "runtime"), Ports{Postgres: 15432, OpenBao: 15432}); err == nil {
		t.Fatal("expected duplicate host port rejection")
	}
}

func TestDefaultStateDirUsesUserDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "baseharbor", "runtime")
	if dir != want {
		t.Fatalf("state dir = %q, want %q", dir, want)
	}
}

func TestDefaultStateDirUsesXDGDataHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, _ := os.Getwd()
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "baseharbor", "runtime")
	if dir != want {
		t.Fatalf("state dir = %q, want %q", dir, want)
	}
}

func TestStateDirOverrideWins(t *testing.T) {
	override := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("BASEHARBOR_STATE_DIR", override)
	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dir != override {
		t.Fatalf("state dir = %q, want override %q", dir, override)
	}
}

func TestLegacyStateIsReusedWhenGlobalStateIsAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	old, _ := os.Getwd()
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	legacy := filepath.Join(work, legacyStateDir)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, envName), []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := resolveStateDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dir != legacyStateDir {
		t.Fatalf("state dir = %q, want legacy %q", dir, legacyStateDir)
	}
}

func TestEmbeddedComposeUsesOneSharedSpiloDCS(t *testing.T) {
	text := string(composeYAML)
	const hosts = `      ETCD3_HOSTS: "'postgres-etcd-1:2379','postgres-etcd-2:2379','postgres-etcd-3:2379'"`
	if got := strings.Count(text, hosts); got != 3 {
		t.Fatalf("Spilo ETCD3_HOSTS appears %d times, want exactly 3", got)
	}
	const scope = "      PATRONI_SCOPE: baseharbor-control-postgres\n"
	if got := strings.Count(text, scope); got != 3 {
		t.Fatalf("PATRONI_SCOPE appears %d times, want exactly 3", got)
	}
	if strings.Contains(text, "ETCD3_HOSTS:") {
		t.Fatal("Spilo runtime must use ETCD3_HOSTS so /launch.sh configures one shared DCS")
	}
}

func TestEmbeddedComposeUsesNativeTLSFromFirstStart(t *testing.T) {
	text := string(composeYAML)
	for _, want := range []string{
		"ghcr.io/zalando/spilo-18:4.1-p2",
		"gcr.io/etcd-development/etcd:v3.7.2",
		"SSL_CERTIFICATE_FILE: /run/baseharbor/tls/server-cert.pem",
		"SSL_PRIVATE_KEY_FILE: /run/baseharbor/tls-runtime/server-key.pem",
		"install -d -o postgres -g postgres -m 0700 /run/baseharbor/tls-runtime",
		"install -o postgres -g postgres -m 0600 /run/baseharbor/tls-source/server-key.pem /run/baseharbor/tls-runtime/server-key.pem",
		`until /bin/sh /run/baseharbor/openbao-init.sh; do`,
		`attempts=$((attempts+1))`,
		`if [ "${attempts}" -ge 90 ]; then`,
		"BAO_ADDR: https://127.0.0.1:8200",
		"openbao-member-1",
		"command: [\"server\", \"-config=/run/baseharbor/openbao/openbao.hcl\"]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("embedded runtime missing native-TLS bootstrap %q", want)
		}
	}
	for _, forbidden := range []string{
		"command: [\"server\", \"-dev",
		"openbao server -dev",
		"bao server -dev",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("openbao must not run in dev mode: found %q", forbidden)
		}
	}
}

func TestEmbeddedComposeUsesOpenBaoPostgreSQLStorage(t *testing.T) {
	text := string(composeYAML)
	for _, wanted := range []string{
		"docker.io/openbao/openbao:2.7.0",
		"command: [\"server\", \"-config=/run/baseharbor/openbao/openbao.hcl\"]",
		"BASEHARBOR_OPENBAO_DB_PASSWORD",
		"./providers/postgresql/runtime/openbao-init.sh:/run/baseharbor/openbao-init.sh:ro",
		"./providers/postgresql/runtime/ca.pem:/run/baseharbor/postgres-ca/ca.pem:ro",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("managed OpenBao/PostgreSQL runtime missing %q", wanted)
		}
	}
	for _, forbidden := range []string{"openbao-data:", "/openbao/raft", "/openbao/file"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("obsolete OpenBao storage remains in current runtime: %q", forbidden)
		}
	}
}

func TestDataDirDoesNotSwitchLegacyRuntimeSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	legacy := filepath.Join(work, legacyStateDir)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, envName), []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dataDir, err := DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dataDir != ".baseharbor" {
		t.Fatalf("data dir = %q, want .baseharbor", dataDir)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "provider-registry.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtimeDir, err := StateDir("")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeDir != legacyStateDir {
		t.Fatalf("runtime dir switched to %q, want legacy %q", runtimeDir, legacyStateDir)
	}

	global, err := globalStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(global); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("global runtime directory unexpectedly materialized: %v", err)
	}
}

func TestDataDirKeepsExplicitOverrideSelfContained(t *testing.T) {
	override := filepath.Join(t.TempDir(), "baseharbor-runtime")
	t.Setenv("BASEHARBOR_STATE_DIR", override)

	dataDir, err := DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	if dataDir != override {
		t.Fatalf("data dir = %q, want override %q", dataDir, override)
	}
}

func TestEmbeddedComposeRunsControlPlaneServicesUnprivileged(t *testing.T) {
	text := string(composeYAML)
	for _, want := range []string{
		"user: \"99:99\"",
		"user: \"100\"",
		"SKIP_CHOWN: \"1\"",
		"/openbao/config:rw,noexec,nosuid,nodev,mode=1777",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"/var/run/postgresql:rw,noexec,nosuid,nodev",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("embedded runtime compose missing %q:\n%s", want, text)
		}
	}
}

func TestEmbeddedComposeUsesStablePatroniMemberIdentitiesAcrossRecreate(t *testing.T) {
	text := string(composeYAML)
	for ordinal := 1; ordinal <= 3; ordinal++ {
		member := fmt.Sprintf("postgres-member-%d", ordinal)
		for _, want := range []string{
			"  " + member + ":\n",
			"    hostname: " + member + "\n",
			"      PATRONI_NAME: " + member + "\n",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("embedded runtime missing stable Patroni identity %q", want)
			}
		}
		if got := strings.Count(text, "      PATRONI_NAME: "+member+"\n"); got != 1 {
			t.Fatalf("embedded runtime PATRONI_NAME for %s appears %d times, want exactly 1", member, got)
		}
	}
}
