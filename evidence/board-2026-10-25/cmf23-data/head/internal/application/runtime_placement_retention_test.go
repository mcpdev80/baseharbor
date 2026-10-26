package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// dedicatedSQLManifest pins application-scoped PostgreSQL placement.
func dedicatedSQLManifest(t *testing.T, name string) Manifest {
	t.Helper()
	m := New(name, "dev", true, false, false)
	m.backendPlacements = map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	}
	return m
}

// sharedSQLManifest pins native-shared PostgreSQL placement.
func sharedSQLManifest(t *testing.T, name string) Manifest {
	t.Helper()
	m := New(name, "dev", true, false, false)
	m.backendPlacements = map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	}
	return m
}

func writePlacementRuntimeEnv(t *testing.T, path string, m Manifest, values map[string]string) {
	t.Helper()
	if err := writeRuntimeEnv(path, m, values); err != nil {
		t.Fatal(err)
	}
}

// simulateSharedBackendProjection mirrors what ReconcileSharedBackends writes
// to the application runtime environment during a shared placement phase:
// sharedPostgresRoleName-derived identifiers plus a provider-managed shared
// password in the consumed slots (internal/application/shared_backends.go).
func simulateSharedBackendProjection(t *testing.T, path string, m Manifest, instance string) (string, string, string) {
	t.Helper()
	values, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	sharedDB := "baha_shared_era_db"
	sharedUser := "baha_shared_era_user"
	sharedPass := "shared-era-password-material"
	values[postgresRuntimeKey(instance, "DB")] = sharedDB
	values[postgresRuntimeKey(instance, "USER")] = sharedUser
	values[postgresRuntimeKey(instance, "PASSWORD")] = sharedPass
	writePlacementRuntimeEnv(t, path, m, values)
	return sharedDB, sharedUser, sharedPass
}

func readPlacementRuntimeEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	values, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// The f64 blocker: dedicated -> shared -> dedicated switch-back must return
// the native consumed slots to the original dedicated material. Bindings and
// the consumed slots are both poisoned by the shared phase (f65 dump:
// shared_backends.go writes runtime.env and projects the same values into
// bindings/), so only the protected owner slots can restore them.
func TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials(t *testing.T) {
	name := "switch-back"
	mDedicated := dedicatedSQLManifest(t, name)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, mDedicated); err != nil {
		t.Fatal(err)
	}
	original := readPlacementRuntimeEnv(t, path)
	originalPassword := original[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")]
	if originalPassword == "" {
		t.Fatal("dedicated apply must materialize a password")
	}
	if original[postgresRuntimeKey(defaultServiceInstance, "APPLICATION_SECRET")] != originalPassword {
		t.Fatal("first dedicated apply must capture the credential material in its owner slot")
	}

	mShared := sharedSQLManifest(t, name)
	if !UsesSharedPostgreSQL(mShared) {
		t.Fatal("shared manifest must report shared PostgreSQL placement")
	}
	if err := ensureRuntimeEnv(path, mShared); err != nil {
		t.Fatal(err)
	}
	captured := readPlacementRuntimeEnv(t, path)
	if captured[postgresRuntimeKey(defaultServiceInstance, "APPLICATION_SECRET")] != originalPassword {
		t.Fatal("shared-phase apply must preserve the dedicated owner slots")
	}
	sharedDB, sharedUser, sharedPass := simulateSharedBackendProjection(t, path, mShared, defaultServiceInstance)

	// Switch back: ensureRuntimeEnv for the dedicated manifest must restore DB,
	// USER and PASSWORD from the protected owner slots, not from the poisoned
	// bindings or consumed slots.
	if err := ensureRuntimeEnv(path, mDedicated); err != nil {
		t.Fatal(err)
	}
	after := readPlacementRuntimeEnv(t, path)
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] == sharedDB {
		t.Errorf("DB still carries the shared-era value %q", sharedDB)
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] == sharedUser {
		t.Errorf("USER still carries the shared-era value %q", sharedUser)
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != postgresDatabaseName(mDedicated, defaultServiceInstance) {
		t.Errorf("DB = %q, want dedicated database", after[postgresRuntimeKey(defaultServiceInstance, "DB")])
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "USER")] != "baseharbor" {
		t.Errorf("USER = %q, want dedicated baseharbor role", after[postgresRuntimeKey(defaultServiceInstance, "USER")])
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] != originalPassword {
		t.Error("PASSWORD was not restored to the material the retained volume authenticates against")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] == sharedPass {
		t.Error("PASSWORD still carries the shared-era material")
	}
}

// Placement-unchanged dedicated reapplies must never rotate or replace
// credential material (existing retention guarantee).
func TestUnchangedDedicatedReapplyDoesNotRotateCredentials(t *testing.T) {
	m := dedicatedSQLManifest(t, "stable")
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	before := readPlacementRuntimeEnv(t, path)
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	after := readPlacementRuntimeEnv(t, path)
	for _, key := range []string{
		postgresRuntimeKey(defaultServiceInstance, "DB"),
		postgresRuntimeKey(defaultServiceInstance, "USER"),
		postgresRuntimeKey(defaultServiceInstance, "PASSWORD"),
		postgresRuntimeKey(defaultServiceInstance, "APPLICATION_DATABASE"),
		postgresRuntimeKey(defaultServiceInstance, "APPLICATION_ROLE"),
		postgresRuntimeKey(defaultServiceInstance, "APPLICATION_SECRET"),
	} {
		if after[key] != before[key] {
			t.Errorf("%s changed across an unchanged-placement reapply", key)
		}
	}
}

// Shared placement must not redirect the consumed POSTGRES_* credential slots
// through the native projection: the shared phase legitimately consumes the
// provider-managed identity until the manifest returns to dedicated placement.
func TestSharedPhaseNeverRewritesConsumedSlotsViaOwnerLogic(t *testing.T) {
	name := "shared-only"
	mShared := sharedSQLManifest(t, name)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, mShared); err != nil {
		t.Fatal(err)
	}
	first := readPlacementRuntimeEnv(t, path)
	simulateSharedBackendProjection(t, path, mShared, defaultServiceInstance)
	if err := ensureRuntimeEnv(path, mShared); err != nil {
		t.Fatal(err)
	}
	after := readPlacementRuntimeEnv(t, path)
	if after[postgresRuntimeKey(defaultServiceInstance, "DB")] != "baha_shared_era_db" {
		t.Error("shared manifest must keep consuming the shared-era slot content")
	}
	if after[postgresRuntimeKey(defaultServiceInstance, "APPLICATION_SECRET")] != first[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] {
		t.Error("first shared apply must have captured the pre-shared material into the owner slot")
	}
}

// Applications without SQL instances are untouched by the projection.
func TestCredentialOwnerSlotsSkippedWhenNoSQLInstances(t *testing.T) {
	m := New("no-sql", "dev", false, false, false)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "POSTGRES") {
		t.Fatal("no-sql manifest must not gain PostgreSQL keys")
	}
}

// Named instances keep their owner slots per instance.
func TestNamedSQLInstanceRoundTripKeepsOwnOwnerSlots(t *testing.T) {
	name := "multi-sql"
	mDedicated := WithSQLInstances(dedicatedSQLManifest(t, name), defaultServiceInstance, "analytics")
	if len(SQLInstanceNames(mDedicated)) != 2 {
		t.Fatalf("test manifest needs two SQL instances, got %v", SQLInstanceNames(mDedicated))
	}
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "runtime.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, mDedicated); err != nil {
		t.Fatal(err)
	}
	original := readPlacementRuntimeEnv(t, path)

	mShared := WithSQLInstances(sharedSQLManifest(t, name), defaultServiceInstance, "analytics")
	if err := ensureRuntimeEnv(path, mShared); err != nil {
		t.Fatal(err)
	}
	simulateSharedBackendProjection(t, path, mShared, "analytics")

	if err := ensureRuntimeEnv(path, mDedicated); err != nil {
		t.Fatal(err)
	}
	after := readPlacementRuntimeEnv(t, path)
	for _, key := range []string{
		postgresRuntimeKey("analytics", "DB"),
		postgresRuntimeKey("analytics", "USER"),
		postgresRuntimeKey("analytics", "PASSWORD"),
	} {
		if after[key] != original[key] {
			t.Errorf("%s not restored to the dedicated material", key)
		}
	}
	if after[postgresRuntimeKey("analytics", "APPLICATION_SECRET")] != original[postgresRuntimeKey("analytics", "PASSWORD")] {
		t.Error("named instance owner slot lost the original material")
	}
}
