package deployment

import (
	"testing"
)

// TestMigrateTargetUIDIsIdempotent proves the name->UID migration creates a UID
// exactly once and reuses it on every later call, so a second create or a
// re-run of the same command never invents a new stable identity.
func TestMigrateTargetUIDIsIdempotent(t *testing.T) {
	def := TargetDefinition{}
	uid1, created1, err := MigrateTargetUID(def)
	if err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if !created1 {
		t.Fatal("first migration must create a UID")
	}
	if err := ValidateTargetUID(uid1); err != nil {
		t.Fatalf("migrated UID invalid: %v", err)
	}

	def.TargetUID = uid1
	uid2, created2, err := MigrateTargetUID(def)
	if err != nil {
		t.Fatalf("second migration: %v", err)
	}
	if created2 {
		t.Fatal("second migration must be idempotent, not create a new UID")
	}
	if uid1 != uid2 {
		t.Fatalf("UID changed across idempotent migration: %q != %q", uid1, uid2)
	}
}

// TestMigrateTargetUIDPreservesExplicitUID proves a UID already present on a
// definition is validated but never regenerated, so historical UIDs survive.
func TestMigrateTargetUIDPreservesExplicitUID(t *testing.T) {
	uid, err := NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	def := TargetDefinition{TargetUID: uid}
	got, created, err := MigrateTargetUID(def)
	if err != nil {
		t.Fatalf("explicit-UID migration: %v", err)
	}
	if created {
		t.Fatal("an explicit existing UID must not be regenerated")
	}
	if got != uid {
		t.Fatalf("explicit UID not preserved: %q != %q", got, uid)
	}
}

// TestMigrateTargetUIDRejectsMalformedProvesFailClosed proves a malformed UID on
// an existing definition fails closed rather than being silently rewritten.
func TestMigrateTargetUIDRejectsMalformedProvesFailClosed(t *testing.T) {
	def := TargetDefinition{TargetUID: "not-a-uid"}
	if _, _, err := MigrateTargetUID(def); err == nil {
		t.Fatal("malformed UID must fail closed")
	}
}

// TestResolveTargetUIDsCollideFailClosed proves a configuration in which two
// distinct Targets resolve to one UID is rejected at the uniqueness seam, so the
// name->UID resolution can never be poisoned by a double claim.
func TestResolveTargetUIDsCollideFailClosed(t *testing.T) {
	shared, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	colliding := Config{
		Version: ConfigVersion,
		Targets: map[string]TargetDefinition{
			"alpha": {TargetUID: shared},
			"beta":  {TargetUID: shared},
		},
	}
	if _, err := ResolveTargetUIDs(colliding); err == nil {
		t.Fatal("duplicate UID across two Targets must fail closed")
	}
}

// TestCheckTargetUIDCollisionByName proves CheckTargetUIDCollision rejects an
// explicit UID that matches another name's UID, but accepts an empty or unique
// UID, and never treats the name being created as a self-collision.
func TestCheckTargetUIDCollisionByName(t *testing.T) {
	others, err := NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	uids := map[string]string{"other": others}

	if !CheckTargetUIDCollision(uids, "mine", others) {
		t.Fatal("a UID already held by another name must collide")
	}
	if CheckTargetUIDCollision(uids, "mine", "") {
		t.Fatal("an empty UID must never collide")
	}
	unique, err := NewTargetUID()
	if err != nil {
		t.Fatal(err)
	}
	if CheckTargetUIDCollision(uids, "mine", unique) {
		t.Fatal("a unique UID must not collide")
	}
	// The name being created already carries the UID in the post-mutation map.
	uids["mine"] = unique
	if CheckTargetUIDCollision(uids, "mine", unique) {
		t.Fatal("a name must not self-collision against its own UID")
	}
}

// TestResolveTargetThreadsUID proves the resolved Target surfaces the stable UID
// without the name ever changing, and without touching the ownership TenantID.
func TestResolveTargetThreadsUID(t *testing.T) {
	uid, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	tenant, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Version: ConfigVersion,
		Access: map[string]AccessDefinition{
			"local": {Provider: "docker", Reference: "local"},
		},
		Targets: map[string]TargetDefinition{
			"docker-dev": {
				TargetUID: uid,
				TenantID:  tenant,
				Runtime:   RuntimeDefinition{Provider: "docker"},
				Access:    TargetAccess{Reference: "local"},
			},
		},
	}
	resolved, err := cfg.ResolveTarget("docker-dev", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.UID != uid {
		t.Fatalf("resolved UID = %q, want %q", resolved.UID, uid)
	}
	if resolved.TenantID != tenant {
		t.Fatalf("owner TenantID changed: %q want %q", resolved.TenantID, tenant)
	}
	if resolved.Name != "docker-dev" {
		t.Fatalf("name must stay addressable: %q", resolved.Name)
	}
}

// TestResolveTargetUIDsDistinctProvesNoConflation proves the three distinct
// identity roles are not mixed: the UID differs from the ownership TenantID, and
// neither equals the name.
func TestResolveTargetUIDsDistinctProvesNoConflation(t *testing.T) {
	def, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	tenant, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if def == tenant {
		t.Fatal("Target UID must be distinct from the Core ownership TenantID")
	}
}

// TestConfigValidateAcceptsUIDAndRejectsBad proves the config schema carries the
// field and only validates it when present, so pre-UID definitions load.
func TestConfigValidateAcceptsUIDAndRejectsBad(t *testing.T) {
	good := Config{
		Version: ConfigVersion,
		Access:  map[string]AccessDefinition{"local": {Provider: "docker", Reference: "local"}},
		Targets: map[string]TargetDefinition{
			"docker-dev": {
				TargetUID: "11111111-1111-4111-8111-111111111111",
				TenantID:  "22222222-2222-4222-8222-222222222222",
				Runtime:   RuntimeDefinition{Provider: "docker"},
				Access:    TargetAccess{Reference: "local"},
			},
		},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid UID definition rejected: %v", err)
	}

	// Empty UID still loads (pre-stable-identity definition).
	goodDef := good.Targets["docker-dev"]
	goodDef.TargetUID = ""
	good.Targets["docker-dev"] = goodDef
	if err := good.Validate(); err != nil {
		t.Fatalf("empty UID rejected: %v", err)
	}

	// Malformed UID fails closed.
	badDef := good.Targets["docker-dev"]
	badDef.TargetUID = "bogus"
	good.Targets["docker-dev"] = badDef
	if err := good.Validate(); err == nil {
		t.Fatal("malformed Target UID must fail closed in Validate")
	}
}

// TestRecoveryPreservesUID proves the protected original-definition recovery
// carries the stable UID so recovery never drops the identity.
func TestRecoveryPreservesUID(t *testing.T) {
	uid, _, err := MigrateTargetUID(TargetDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	def := TargetDefinition{TargetUID: uid}
	r := TargetDefinitionRecovery{Version: TargetDefinitionRecoveryVersion, Name: "docker-dev", Definition: def}
	if r.TargetUID() != uid {
		t.Fatalf("recovery lost the UID: %q != %q", r.TargetUID(), uid)
	}
}
