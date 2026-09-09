package application

import (
	"path/filepath"
	"testing"
)

func TestReallocateRuntimePortsPreservesCredentials(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("port-retry", "dev", true, true, false)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}

	before, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	beforePostgresPort := before[postgresRuntimeKey(defaultServiceInstance, "HOST_PORT")]
	beforeValkeyPort := before[valkeyRuntimeKey(defaultServiceInstance, "HOST_PORT")]
	beforePostgresPassword := before[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")]
	beforeValkeyPassword := before[valkeyRuntimeKey(defaultServiceInstance, "PASSWORD")]
	beforeDatabase := before[postgresRuntimeKey(defaultServiceInstance, "DB")]
	beforeUser := before[postgresRuntimeKey(defaultServiceInstance, "USER")]

	if err := ReallocateRuntimePorts(m, files); err != nil {
		t.Fatal(err)
	}

	after, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "HOST_PORT")] == beforePostgresPort {
		t.Fatal("PostgreSQL host port was not replaced")
	}
	if after[valkeyRuntimeKey(defaultServiceInstance, "HOST_PORT")] == beforeValkeyPort {
		t.Fatal("Valkey host port was not replaced")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] != beforePostgresPassword {
		t.Fatal("PostgreSQL password changed during port reallocation")
	}
	if after[valkeyRuntimeKey(defaultServiceInstance, "PASSWORD")] != beforeValkeyPassword {
		t.Fatal("Valkey password changed during port reallocation")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != beforeDatabase {
		t.Fatal("PostgreSQL database changed during port reallocation")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] != beforeUser {
		t.Fatal("PostgreSQL user changed during port reallocation")
	}

	contractValues, err := readRuntimeEnv(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if contractValues["DATABASE_URL"] == "" || contractValues["REDIS_URL"] == "" {
		t.Fatal("application environment contract was not refreshed")
	}
}
