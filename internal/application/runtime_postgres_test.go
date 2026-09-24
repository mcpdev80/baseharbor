package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestEnsureRuntimePostgresIsolatedAndIdempotent(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}

	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), `127.0.0.1:${POSTGRES_HOST_PORT}:5432`) {
		t.Fatal("application postgres must be published only on loopback")
	}
	if !strings.Contains(string(compose), "postgres-data:/var/lib/postgresql") {
		t.Fatal("dedicated postgres volume is missing or uses the pre-18 mount path")
	}
	if strings.Contains(string(compose), "postgres-data:/var/lib/postgresql/data") {
		t.Fatal("PostgreSQL 18 runtime must not use the pre-18 data volume mount")
	}

	firstEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firstEnv), "POSTGRES_PASSWORD=baseharbor") {
		t.Fatal("runtime must use a generated password")
	}
	if !strings.Contains(string(firstEnv), "POSTGRES_HOST_PORT=") {
		t.Fatal("runtime must allocate a local PostgreSQL port")
	}
	if _, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m); err != nil {
		t.Fatal(err)
	}
	secondEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstEnv) != string(secondEnv) {
		t.Fatal("repeated convergence rotated credentials or ports unexpectedly")
	}

	for _, path := range []string{files.Compose, files.Env, files.ApplicationEnv, files.Bindings} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s is accessible by group or others: %o", path, info.Mode().Perm())
		}
	}
}

func TestEnsureRuntimePostgresAndValkey(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, true, true)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, wanted := range []string{
		"docker.io/library/postgres:18-alpine",
		"docker.io/valkey/valkey:9.1.2-alpine",
		"./providers/postgresql/default/runtime/server-cert.pem:/run/baseharbor/tls-source/server-cert.pem:ro",
		"./providers/postgresql/default/runtime/server-key.pem:/run/baseharbor/tls-source/server-key.pem:ro",
		"./providers/postgresql/default/runtime/pg_hba.conf:/run/baseharbor/tls-source/pg_hba.conf:ro",
		"exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on",
		"hba_file=/run/baseharbor/tls-source/pg_hba.conf",
		"./providers/valkey/default/service-access/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro",
		"postgres-data:/var/lib/postgresql",
		"valkey-data:/data",
		"appendonly yes",
		`127.0.0.1:${POSTGRES_HOST_PORT}:5432`,
		`127.0.0.1:${VALKEY_HOST_PORT}:6379`,
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("runtime compose missing %q", wanted)
		}
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"VALKEY_PASSWORD=", "POSTGRES_HOST_PORT=", "VALKEY_HOST_PORT="} {
		if !strings.Contains(string(env), wanted) {
			t.Fatalf("runtime env missing %q", wanted)
		}
	}
}

func TestEnsureRuntimeBackfillsPortsWithoutRotatingCredentials(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("legacy", "dev", true, true, false)
	dir := filepath.Join(store.Root, m.Name, "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := "POSTGRES_DB=legacy_dev\nPOSTGRES_USER=baseharbor\nPOSTGRES_PASSWORD=keep-postgres\nVALKEY_PASSWORD=keep-valkey\n"
	if err := os.WriteFile(filepath.Join(dir, "runtime.env"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, wanted := range []string{"POSTGRES_PASSWORD=keep-postgres", "VALKEY_PASSWORD=keep-valkey", "POSTGRES_HOST_PORT=", "VALKEY_HOST_PORT="} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("upgraded runtime env missing %q", wanted)
		}
	}
}

func TestEnsureRuntimeCreatesNativeApplicationContract(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, true, true)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	appEnv, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	text := string(appEnv)
	for _, wanted := range []string{
		"BASEHARBOR_APP_NAME=demo",
		"BASEHARBOR_ENVIRONMENT=dev",
		"BASEHARBOR_BINDINGS=",
		"DATABASE_URL=postgresql://",
		"REDIS_URL=rediss://",
		"VALKEY_URL=rediss://",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("application contract missing %q", wanted)
		}
	}
	for _, path := range []string{
		filepath.Join(files.Bindings, "postgres", "host"),
		filepath.Join(files.Bindings, "postgres", "port"),
		filepath.Join(files.Bindings, "postgres", "database"),
		filepath.Join(files.Bindings, "postgres", "username"),
		filepath.Join(files.Bindings, "postgres", "password"),
		filepath.Join(files.Bindings, "postgres", "uri"),
		filepath.Join(files.Bindings, "valkey", "host"),
		filepath.Join(files.Bindings, "valkey", "port"),
		filepath.Join(files.Bindings, "valkey", "password"),
		filepath.Join(files.Bindings, "valkey", "uri"),
		filepath.Join(files.Bindings, "metadata.json"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("binding %s: %v", path, err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("binding %s is accessible by group or others: %o", path, info.Mode().Perm())
		}
	}

	metadata, err := os.ReadFile(filepath.Join(files.Bindings, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadataText := string(metadata)
	for _, wanted := range []string{
		"\"kind\": \"database.sql\"",
		"\"provider\": \"postgresql\"",
		"\"kind\": \"cache.key-value\"",
		"\"provider\": \"valkey\"",
		"\"kind\": \"secrets\"",
		"\"provider\": \"openbao\"",
		"\"workload\": \"application/demo\"",
	} {
		if !strings.Contains(metadataText, wanted) {
			t.Fatalf("runtime metadata missing %q:\n%s", wanted, metadataText)
		}
	}
}

func TestEnsureRuntimeCreatesMultipleNamedServiceInstances(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", false, false, false)
	m = WithSQLInstances(m, "primary", "analytics")
	m = WithCacheInstances(m, "cache", "sessions")
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}

	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	for _, wanted := range []string{
		"  postgres-primary:\n",
		"  postgres-analytics:\n",
		"  valkey-cache:\n",
		"  valkey-sessions:\n",
		`127.0.0.1:${POSTGRES_PRIMARY_HOST_PORT}:5432`,
		`127.0.0.1:${POSTGRES_ANALYTICS_HOST_PORT}:5432`,
		`127.0.0.1:${VALKEY_CACHE_HOST_PORT}:6379`,
		`127.0.0.1:${VALKEY_SESSIONS_HOST_PORT}:6379`,
	} {
		if !strings.Contains(composeText, wanted) {
			t.Fatalf("multi-instance compose missing %q:\n%s", wanted, composeText)
		}
	}

	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	ports := map[string]struct{}{}
	for _, key := range []string{"POSTGRES_PRIMARY_HOST_PORT", "POSTGRES_ANALYTICS_HOST_PORT", "VALKEY_CACHE_HOST_PORT", "VALKEY_SESSIONS_HOST_PORT"} {
		value := values[key]
		if value == "" {
			t.Fatalf("runtime env missing %s", key)
		}
		if _, exists := ports[value]; exists {
			t.Fatalf("runtime reused host port %s", value)
		}
		ports[value] = struct{}{}
	}

	appEnv, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	appText := string(appEnv)
	for _, wanted := range []string{
		"DATABASE_URL=postgresql://",
		"DATABASE_PRIMARY_URL=postgresql://",
		"DATABASE_ANALYTICS_URL=postgresql://",
		"REDIS_CACHE_URL=rediss://",
		"REDIS_SESSIONS_URL=rediss://",
		"VALKEY_CACHE_URL=rediss://",
		"VALKEY_SESSIONS_URL=rediss://",
	} {
		if !strings.Contains(appText, wanted) {
			t.Fatalf("multi-instance application contract missing %q:\n%s", wanted, appText)
		}
	}
	if strings.Contains(appText, "\nREDIS_URL=") || strings.Contains(appText, "\nVALKEY_URL=") {
		t.Fatalf("ambiguous generic Redis URL must not be emitted for cache+sessions:\n%s", appText)
	}

	for _, path := range []string{
		filepath.Join(files.Bindings, "postgres", "primary", "uri"),
		filepath.Join(files.Bindings, "postgres", "analytics", "uri"),
		filepath.Join(files.Bindings, "valkey", "cache", "uri"),
		filepath.Join(files.Bindings, "valkey", "sessions", "uri"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("named binding %s: %v", path, err)
		}
	}
}

func TestAddingNamedInstanceDoesNotRotateExistingInstance(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", false, false, false)
	m = WithSQLInstances(m, "primary")
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	before, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	password := before["POSTGRES_PRIMARY_PASSWORD"]
	port := before["POSTGRES_PRIMARY_HOST_PORT"]

	m = WithSQLInstances(m, "analytics")
	if _, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m); err != nil {
		t.Fatal(err)
	}
	after, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if after["POSTGRES_PRIMARY_PASSWORD"] != password || after["POSTGRES_PRIMARY_HOST_PORT"] != port {
		t.Fatal("adding a named PostgreSQL instance rotated the existing primary instance")
	}
	if after["POSTGRES_ANALYTICS_PASSWORD"] == "" || after["POSTGRES_ANALYTICS_HOST_PORT"] == "" {
		t.Fatal("new analytics instance was not materialized")
	}
}

func TestEnsureRuntimeValkeyOnly(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("cache", "dev", false, true, false)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compose), "postgres:") {
		t.Fatal("Valkey-only runtime unexpectedly contains PostgreSQL")
	}
	if !strings.Contains(string(compose), "docker.io/valkey/valkey:9.1.2-alpine") {
		t.Fatal("Valkey-only runtime is missing Valkey")
	}
}

func TestEnsureRuntimeAllowsSecretsAlongsideMaterializedService(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, true)
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "docker.io/library/postgres:18-alpine") {
		t.Fatal("managed secrets changed the PostgreSQL runtime definition")
	}
}

func TestEnsureRuntimeRejectsSecretsOnlyUntilStandaloneLifecycleExists(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("secret-only", "dev", false, false, true)
	if _, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m); err == nil {
		t.Fatal("expected secrets-only runtime to fail closed")
	}
}

func TestRuntimeProjectNameIncludesApplicationAndEnvironment(t *testing.T) {
	m := New("mailflow", "prod", true, false, false)
	if got := RuntimeProjectName(m); got != "baseharbor-mailflow-prod" {
		t.Fatalf("unexpected project name %q", got)
	}
}

func TestManagedDatabaseAndCacheComposeAreUnprivileged(t *testing.T) {
	m := New("demo", "dev", true, true, false)
	got, err := RuntimeComposeYAML(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"user: \"postgres\"",
		"user: \"valkey\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"/var/run/postgresql:rw,noexec,nosuid,nodev",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("managed runtime compose missing %q:\n%s", want, got)
		}
	}
}
