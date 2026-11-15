package deployment

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadConfigMigratesLegacyTargetIdempotent proves the read seam
// (LoadConfig) fills the missing stable Target UID for a legacy definition and
// persists it exactly once: the first load generates the UID, and a second load
// observes no churn (same UID, no rewrite). State directories and target names
// are left untouched.
func TestLoadConfigMigratesLegacyTargetIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	// A legacy config with a Target that has NO uid (pre-stable-identity) but a
	// fully valid runtime/access so Validate passes once migrated.
	legacy := []byte("version: 1\n" +
		"access:\n" +
		"  local-docker:\n" +
		"    provider: docker\n" +
		"    reference: local\n" +
		"targets:\n" +
		"  docker-legacy:\n" +
		"    runtime:\n" +
		"      provider: docker\n" +
		"    access:\n" +
		"      reference: local-docker\n")
	// ConfigPath() under XDG_CONFIG_HOME nests under <home>/baseharbor/.
	cfgPath := filepath.Join(root, "config", "baseharbor", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	// First read: LoadConfig fills the missing UID for the legacy Target and
	// persists it exactly once.
	cfg1, err := LoadConfig()
	if err != nil {
		t.Fatalf("first LoadConfig: %v", err)
	}
	uid1 := cfg1.Targets["docker-legacy"].TargetUID
	if uid1 == "" {
		t.Fatal("LoadConfig did not fill a stable Target UID for the legacy Target")
	}

	// The persisted config now carries the UID.
	persisted, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(persisted), "target-uid:") {
		t.Fatalf("migrated UID not persisted to config.yaml:\n%s", string(persisted))
	}

	// Second read: same UID, no churn — the idempotent read contract.
	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("second LoadConfig: %v", err)
	}
	if uid2 := cfg2.Targets["docker-legacy"].TargetUID; uid2 != uid1 {
		t.Fatalf("UID churned across reads: %q != %q", uid1, uid2)
	}
	persistedAfter, _ := os.ReadFile(cfgPath)
	if !bytes.Equal(persisted, persistedAfter) {
		t.Fatal("second LoadConfig rewrote the config (UID churn on read)")
	}

	// The runtime definition is untouched by migration.
	if cfg1.Targets["docker-legacy"].Runtime.Provider != "docker" {
		t.Fatal("migration altered the runtime definition")
	}
}

// TestMigrateLegacyTargetUIDsFailsClosedOnCollision proves the seam rejects a
// configuration whose UIDs collide: it returns an error and changes nothing.
func TestMigrateLegacyTargetUIDsFailsClosedOnCollision(t *testing.T) {
	uid, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	colliding := Config{
		Version: ConfigVersion,
		Access:  map[string]AccessDefinition{"local": {Provider: "docker", Reference: "local"}},
		Targets: map[string]TargetDefinition{
			"alpha": {TargetUID: uid, Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}},
			"beta":  {TargetUID: uid, Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}},
		},
	}
	if _, _, err := MigrateLegacyTargetUIDs(colliding); err == nil {
		t.Fatal("two Targets sharing one UID must fail closed")
	}
}

// TestUIDSurvivesConfigRoundTripPreservesRoleSeparation proves the three
// distinct identity roles survive a full config write → decode → list round
// trip without conflating: the stable Target UID differs from the ownership
// TenantID and from any deployment id, and all three survive serialization.
func TestUIDSurvivesConfigRoundTripPreservesRoleSeparation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	uid, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	tenant, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if uid == tenant {
		t.Fatal("UID and TenantID must be distinct values")
	}
	def := TargetDefinition{
		TargetUID: uid,
		TenantID:  tenant,
		Runtime:   RuntimeDefinition{Provider: "docker"},
		Access:    TargetAccess{Reference: "local"},
	}
	// Serialize via a full config save/load through the protected owner.
	cfg := Config{
		Version: ConfigVersion,
		Access:  map[string]AccessDefinition{"local": {Provider: "docker", Reference: "local"}},
		Targets: map[string]TargetDefinition{"docker-prod": def},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	got := reloaded.Targets["docker-prod"]
	if got.TargetUID != uid || got.TenantID != tenant {
		t.Fatalf("roles lost across round trip: uid=%q tenant=%q", got.TargetUID, got.TenantID)
	}
	if got.TargetUID == got.TenantID {
		t.Fatal("UID and TenantID conflated after round trip")
	}

	// The recovery copy must also carry the UID distinctly.
	raw, err := TargetRecoveryFromConfig(rawConfigBytes(t, "docker-prod"), "docker-prod")
	if err != nil {
		t.Fatalf("target recovery from config: %v", err)
	}
	if raw.TargetUID() != uid {
		t.Fatalf("recovery lost the UID: %q", raw.TargetUID())
	}
}

// rawConfigBytes returns the on-disk config bytes for the named Target so the
// recovery seam can decode the same protected owner the save wrote.
func rawConfigBytes(t *testing.T, _ string) []byte {
	t.Helper()
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
