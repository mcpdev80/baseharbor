package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestResetPlacementStaleCredentialsOnDedicatedSwitchBack(t *testing.T) {
	// Scenario: dedicated app -> shared backend -> dedicated switch-back
	// The shared-era values in runtime.env should be replaced with bindings/ values.
	m := New("switch-test", "dev", true, false, false)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	bindingsDir := filepath.Join(root, "runtime", "bindings")

	// 1. Initial dedicated apply creates correct bindings.
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}

	// 2. Simulate shared-era credentials: write shared-era values to runtime.env
	//    and create bindings with dedicated values.
	sharedDB := "baha_switch_test_dev_56d7c895"
	sharedUser := "baha_switch_test_dev_6db81550"
	sharedPass := "shared-era-password-xxxx"
	currentValues, _ := readRuntimeEnv(path)
	currentValues[postgresRuntimeKey(defaultServiceInstance, "DB")] = sharedDB
	currentValues[postgresRuntimeKey(defaultServiceInstance, "USER")] = sharedUser
	currentValues[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] = sharedPass
	currentValues[postgresRuntimeKey(defaultServiceInstance, "HOST_PORT")] = "5432"
	writeRuntimeEnv(path, m, currentValues)

	// 3. Create dedicated bindings/ directory with correct values.
	//    These represent the actual dedicated role/database on the volume.
	//    For the default instance, files go directly under bindings/postgres/
	postgresBindingsDir := filepath.Join(bindingsDir, "postgres")
	if err := os.MkdirAll(postgresBindingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	dedicatedDB := "switch_test_dev"
	dedicatedUser := "baseharbor"
	dedicatedPass := "dedicated-password-xxxx"
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "database"), []byte(dedicatedDB+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "username"), []byte(dedicatedUser+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "password"), []byte(dedicatedPass+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// 4. Re-run ensureRuntimeEnv with dedicated placement.
	//    The shared-era values should be replaced with bindings/ values.
	//    Force application/dedicated placement for PostgreSQL to avoid default shared.
	m2 := New("switch-test", "dev", false, false, false)
	m2.Services.SQL = true
	m2.backendPlacements = map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	}
	t.Logf("Before ensureRuntimeEnv: DB=%q, USER=%q",
		currentValues[postgresRuntimeKey(defaultServiceInstance, "DB")],
		currentValues[postgresRuntimeKey(defaultServiceInstance, "USER")])
	if err := ensureRuntimeEnv(path, m2); err != nil {
		t.Fatal(err)
	}

	after, err := readRuntimeEnv(path)
	// Debug: print actual values
	t.Logf("After ensureRuntimeEnv: DB=%q, USER=%q, PASS len=%d",
		after[postgresRuntimeKey(defaultServiceInstance, "DB")],
		after[postgresRuntimeKey(defaultServiceInstance, "USER")],
		len(after[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")]))
	t.Logf("SQLInstanceNames(m2)=%v, UsesSharedPostgreSQL(m2)=%v",
		SQLInstanceNames(m2),
		UsesSharedPostgreSQL(m2))
	if err != nil {
		t.Fatal(err)
	}

	// 5. Verify: DB and USER should now match bindings/ values, not shared-era.
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != dedicatedDB {
		t.Errorf("DB not replaced: got %q (stale: %q, expected: %q)",
			after[postgresRuntimeKey(defaultServiceInstance, "DB")], sharedDB, dedicatedDB)
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] != dedicatedUser {
		t.Errorf("USER not replaced: got %q (stale: %q, expected: %q)",
			after[postgresRuntimeKey(defaultServiceInstance, "USER")], sharedUser, dedicatedUser)
	}
}

func TestResetPlacementStaleCredentialsDoesNotAffectUnchangedDedicated(t *testing.T) {
	// When placement is already dedicated and bindings match runtime.env,
	// no values should be overwritten.
	m := New("stable-test", "dev", true, false, false)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	bindingsDir := filepath.Join(root, "runtime", "bindings")

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}

	original, _ := readRuntimeEnv(path)

	// Create bindings/ with values matching current runtime.env.
	postgresBindingsDir := filepath.Join(bindingsDir, "postgres")
	if err := os.MkdirAll(postgresBindingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	currentDB := original[postgresRuntimeKey(defaultServiceInstance, "DB")]
	currentUser := original[postgresRuntimeKey(defaultServiceInstance, "USER")]
	currentPass := original[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")]
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "database"), []byte(currentDB+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "username"), []byte(currentUser+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "password"), []byte(currentPass+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Re-run with dedicated placement.
	m2 := New("stable-test", "dev", false, false, false)
	m2.Services.SQL = true
	m2.backendPlacements = map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	}
	if err := ensureRuntimeEnv(path, m2); err != nil {
		t.Fatal(err)
	}

	after, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}

	// Values should be unchanged - retention guarantees preserved.
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != original[postgresRuntimeKey(defaultServiceInstance, "DB")] {
		t.Error("DB was unnecessarily changed during unchanged dedicated")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] != original[postgresRuntimeKey(defaultServiceInstance, "USER")] {
		t.Error("USER was unnecessarily changed during unchanged dedicated")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] != original[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] {
		t.Error("PASSWORD was unnecessarily changed during unchanged dedicated")
	}
}

func TestResetPlacementStaleCredentialsSkippedWhenShared(t *testing.T) {
	// When using shared backend, the reset function must NOT run.
	// Shared backend provisioning writes its own values intentionally.
	m := New("shared-test", "dev", false, false, false)
	m.Services.SQL = true
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}

	original, _ := readRuntimeEnv(path)

	// Write shared-era values (these should be preserved when shared is active).
	sharedDB := "baha_shared_test_dev_56d7c895"
	sharedUser := "baha_shared_test_dev_6db81550"
	sharedPass := "shared-era-password"
	currentValues, _ := readRuntimeEnv(path)
	currentValues[postgresRuntimeKey(defaultServiceInstance, "DB")] = sharedDB
	currentValues[postgresRuntimeKey(defaultServiceInstance, "USER")] = sharedUser
	currentValues[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] = sharedPass
	currentValues[postgresRuntimeKey(defaultServiceInstance, "HOST_PORT")] = "5432"
	writeRuntimeEnv(path, m, currentValues)

	// Create bindings with DIFFERENT values (simulating what bindings might have).
	bindingsDir := filepath.Join(root, "runtime", "bindings")
	postgresBindingsDir := filepath.Join(bindingsDir, "postgres")
	if err := os.MkdirAll(postgresBindingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	dedicatedDB := "different-database"
	dedicatedUser := "different-user"
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "database"), []byte(dedicatedDB+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "username"), []byte(dedicatedUser+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postgresBindingsDir, "password"), []byte("different-pass\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Re-run with shared backend.
	// Override placement to use shared backend.
	// UsesSharedPostgreSQL checks ResolveProviderPlacement which checks
	// capability.ProviderPostgreSQL placement.
	// For this test, set the env var to force shared.
	oldScope := os.Getenv(ProviderScopeEnv(capability.ProviderPostgreSQL))
	os.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	defer func() {
		if oldScope != "" {
			os.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), oldScope)
		} else {
			os.Unsetenv(ProviderScopeEnv(capability.ProviderPostgreSQL))
		}
	}()

	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}

	after, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}

	_ = original
	_ = sharedPass
	_ = dedicatedDB
	_ = dedicatedUser
	_ = sharedPass
	// When shared is active, values should NOT be overwritten by bindings/.
	// The reset function skips when UsesSharedPostgreSQL(m) == true.
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != sharedDB {
		t.Errorf("shared-era DB was incorrectly overwritten: got %q, expected %q",
			after[postgresRuntimeKey(defaultServiceInstance, "DB")], sharedDB)
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] != sharedUser {
		t.Errorf("shared-era USER was incorrectly overwritten: got %q, expected %q",
			after[postgresRuntimeKey(defaultServiceInstance, "USER")], sharedUser)
	}
}

func TestResetPlacementStaleCredentialsSkipsWhenNoSQLInstances(t *testing.T) {
	// When there are no SQL instances at all, the function must do nothing.
	m := New("no-sql", "dev", false, false, false)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// No SQL services - just write some random data.
	data := []byte("SOME_OTHER_KEY=some-value\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Re-run without SQL.
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "SOME_OTHER_KEY=some-value") {
		t.Error("file was modified despite no SQL instances")
	}
}
