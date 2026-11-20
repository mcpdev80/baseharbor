package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestLegacyKeycloakMetricsSourceCorpseProvesTheBug fixes the PRE-migration
// behaviour the review demanded as the witness for P4: the unscoped literal
// "keycloak-metrics" is present in the registry file but NOT visible through
// the app-scoped list surface, because the route-2 environment derivation
// cannot resolve an environment from an ID that carries no project. That is
// the "corpse": the record exists yet no owner can ever list or remove it
// through the contract, and the ID-scoping registration drops it silently.
func TestLegacyKeycloakMetricsSourceCorpseProvesTheBug(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	if err := Update(MetricsSource{
		ID:               LegacyKeycloakMetricsID,
		Provider:         capability.ProviderKeycloak,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "alpha",
		Network:          "baseharbor-alpha-dev",
		Target:           "keycloak:9000",
		Path:             "/metrics",
		Scheme:           "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	// Present in the registry (the file and the raw predicate see it).
	if ok, err := RegistryHasSignal(LegacyKeycloakMetricsID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("legacy source not present after its own write")
	}
	// NOT visible through the app-scoped list contract: the unscoped literal
	// fails closed on the environment derivation. This is the loss the
	// migration must make observable.
	list, err := ListMetrics(capability.ProviderPlacement{Scope: capability.ScopeApplication}, []string{"alpha"}, true, true, "dev")
	if err != nil {
		t.Fatalf("ListMetrics: %v", err)
	}
	for _, source := range list {
		if source.ID == LegacyKeycloakMetricsID {
			t.Fatal("the unscoped literal must NOT be listable pre-migration (route-2 fails closed)")
		}
	}
}

// TestMigrateScopedReBooksObservablyForOwner proves the migration half on the
// observability contract: after MigrateScoped the record stands under the
// canonical project-scoped identity, WITH ITS OWNER VERBATIM, resolves an
// environment through the route-2 derivation, IS visible through the app-scoped
// list for its owner, and is removable by that identity. A re-run on the same
// registry changes no byte (idempotent, no-mutation).
func TestMigrateScopedReBooksObservablyForOwner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	if err := Update(MetricsSource{
		ID:               LegacyKeycloakMetricsID,
		Provider:         capability.ProviderKeycloak,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "alpha",
		Network:          "baseharbor-alpha-dev",
		Target:           "keycloak:9000",
		Path:             "/metrics",
		Scheme:           "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	legacy := LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}}
	ownerProjects := map[string]string{"alpha": "bh-target-alpha-identity-dev"}
	migrated, err := legacy.MigrateScoped("bh-target-bravo-identity-dev", ownerProjects)
	if err != nil {
		t.Fatalf("MigrateScoped: %v", err)
	}
	if !migrated {
		t.Fatal("the legacy literal was not re-booked")
	}
	migratedID := "keycloak:bh-target-alpha-dev:keycloak-metrics"
	if ok, err := RegistryHasSignal(LegacyKeycloakMetricsID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("legacy literal still present after migration")
	}
	if ok, err := RegistryHasSource(migratedID, capability.ProviderKeycloak, SignalMetrics, "alpha"); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("migrated record vanished or lost its owner: %q", migratedID)
	}
	source, ok := func() (SignalSource, bool) {
		sources, err := ListAll()
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range sources {
			if candidate.ID == migratedID {
				return candidate, true
			}
		}
		return SignalSource{}, false
	}()
	if !ok {
		t.Fatalf("migrated record missing from the registry: %q", migratedID)
	}
	if env, resolved := SourceEnvironment(source); !resolved || env != "dev" {
		t.Fatalf("the migrated record's environment must resolve to dev, got %q ok=%v", env, resolved)
	}
	list, err := ListMetrics(capability.ProviderPlacement{Scope: capability.ScopeApplication}, []string{"alpha"}, true, true, "dev")
	if err != nil {
		t.Fatalf("ListMetrics: %v", err)
	}
	found := false
	for _, entry := range list {
		if entry.ID == migratedID && entry.OwnerApplication == "alpha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the migrated record is not observable for its owner through the list contract: %#v", list)
	}
	if err := Remove(migratedID); err != nil {
		t.Fatalf("Remove(migrated): %v", err)
	}
	if ok, err := RegistryHasSignal(migratedID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("migrated record still present after Remove by its identity")
	}
}

// TestMigrateScopedIsIdempotentAndWritesNoByte proves P2 on the persistence
// level: a second migration against an already-migrated registry returns
// migrated=false... actually it re-books nothing (no match) and leaves the
// registry file BYTE-IDENTICAL, same mtime-safe content; and a migration that
// finds no legacy literal at all against a MISSING file never creates it.
func TestMigrateScopedIsIdempotentAndWritesNoByte(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	// A MISSING registry file: the change-aware path must not create it.
	legacy := LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}}
	migrated, err := legacy.MigrateScoped("bh-target-bravo-identity-dev")
	if err != nil {
		t.Fatalf("MigrateScoped on a missing registry: %v", err)
	}
	if migrated {
		t.Fatal("nothing exists, so nothing may report as migrated")
	}
	path := filepath.Join(dir, "observability", "signals.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a no-op migration on a missing registry created the file (stat err=%v)", err)
	}
	// Now write a legacy literal, migrate twice, and compare bytes.
	if err := Update(MetricsSource{
		ID: LegacyKeycloakMetricsID, Provider: capability.ProviderKeycloak,
		Class: SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: "alpha", Network: "baseharbor-alpha-dev",
		Target: "keycloak:9000", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	if _, err := legacy.MigrateScoped("bh-target-bravo-identity-dev", map[string]string{"alpha": "bh-target-alpha-identity-dev"}); err != nil {
		t.Fatalf("first MigrateScoped: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first migration: %v", err)
	}
	before := mtimeOf(t, path)
	second, err := legacy.MigrateScoped("bh-target-bravo-identity-dev", map[string]string{"alpha": "bh-target-alpha-identity-dev"})
	if err != nil {
		t.Fatalf("second MigrateScoped: %v", err)
	}
	if second {
		t.Fatal("an already-migrated registry must report migrated=false")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second migration: %v", err)
	}
	if string(first) != string(after) {
		t.Fatalf("a no-op migration rewrote the registry file:\nfirst=%s\nafter=%s", first, after)
	}
	if !mtimeOf(t, path).Equal(before) {
		t.Fatal("a no-op migration replaced the registry file")
	}
}

// mtimeOf reports the modification time of a path, failing the test on error.
// The no-mutation witness compares it across a no-op: a rename-based rewrite
// always advances it.
func mtimeOf(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}

// TestRebookLegacySignalsConvergesCallerOwnRecordToRegistrationID proves the
// canonical-convergence rule (card 0428-168): when the legacy record's owner
// IS the registering application and the caller's project carries the
// application token, the record converges onto the caller's registration
// identity — RebookLegacySignals books it under registrationID, so the
// registration's (ID, provider) replacement then refreshes exactly that entry
// instead of dropping the record. Without the convergence branch the record
// would be re-booked under a derived beside-identity that the replacement
// never selects (leaving a stale duplicate); this test fails on that mutation.
func TestRebookLegacySignalsConvergesCallerOwnRecordToRegistrationID(t *testing.T) {
	recon := LegacyReconciliation{
		Source:            LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}},
		Project:           "bh-target-alpha-identity-dev",
		CallerApplication: "alpha",
		OwnerProjects:     []map[string]string{{"alpha": "bh-target-alpha-identity-dev"}},
	}
	registrationID := "keycloak:bh-target-alpha-dev"
	legacy := SignalSource{
		ID: LegacyKeycloakMetricsID, Kind: SignalMetrics, Provider: capability.ProviderKeycloak,
		Class: SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: "alpha", Network: "baseharbor-alpha-dev",
		Target: "keycloak:9000", Protocol: "openmetrics", Path: "/metrics", Scheme: "http",
	}
	out := RebookLegacySignals(recon, []SignalSource{legacy}, registrationID)
	if len(out) != 1 {
		t.Fatalf("convergence produced %d records, want exactly 1 (the caller's own record must converge, never duplicate)", len(out))
	}
	if out[0].ID != registrationID {
		t.Fatalf("the caller's own legacy record did not converge onto %q: got %q", registrationID, out[0].ID)
	}
	for _, source := range out {
		if source.ID == LegacyKeycloakMetricsID {
			t.Fatal("the legacy literal survived the convergence pass")
		}
	}
}

// TestRebookLegacySignalsPreservesEverySourceField proves the maintainer
// acceptance (PR #1709 comment 7029213393): a re-booked record keeps ALL of
// its source metadata verbatim - network, target, descriptor-carried protocol/
// path/scheme/mode/convention/verification, security, owner, class, scope -
// and only its ID (plus the target-grammar stamp) changes.
func TestRebookLegacySignalsPreservesEverySourceField(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	if err := UpdateSignal(SignalSource{
		ID: LegacyKeycloakMetricsID, Kind: SignalMetrics, Provider: capability.ProviderKeycloak,
		Class: SourcePlatformProvider, Scope: capability.ScopeShared,
		Network: "baseharbor-platform-dev", Target: "keycloak:9000",
		Protocol: "openmetrics", Path: "/custom/metrics", Scheme: "https",
		Mode: capability.ObservabilityNative, SemanticConvention: "otel-v1",
		Verification: capability.ObservabilityVerifyNone,
		Security:     Security{TLSRequired: true, TrustFile: "/trust/ca.pem"},
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	before, err := readSignalsForTest(t)
	if err != nil || len(before) != 1 {
		t.Fatalf("read before: %d sources err=%v", len(before), err)
	}
	recon := LegacyReconciliation{
		Source:        LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}},
		Project:       "bh-target-bravo-identity-dev",
		OwnerProjects: []map[string]string{},
	}
	out := RebookLegacySignals(recon, before, "keycloak:bh-target-bravo-identity-dev")
	if len(out) != 1 {
		t.Fatalf("rebook produced %d records, want 1", len(out))
	}
	got, want := out[0], before[0]
	if got.ID == want.ID {
		t.Fatal("the record was not re-booked")
	}
	got.ID, want.ID = "", ""
	gotTarget, wantTarget := got.Target, want.Target
	got.Target, want.Target = "", ""
	if got != want {
		t.Fatalf("the re-book changed more than the identity:\nbefore=%+v\nafter =%+v", want, got)
	}
	if gotTarget == wantTarget {
		t.Fatal("the environment stamp must repair the bare-endpoint target grammar")
	}
	if _, _, ok := ParseRuntimeTarget(gotTarget); !ok {
		t.Fatalf("stamped target %q is not in the runtime:// grammar", gotTarget)
	}
}

// TestRegisterProviderSignalsRebooksLegacyBeforeReplacement proves P1's core
// end-to-end inside the SAME mutation: with a Legacy reconciliation, the
// (ID, provider) replacement can never overwrite or delete the re-booked
// record; and a caller WITHOUT the reconciliation still drops the literal
// exactly as before (the regression the card exists for is closed by the
// caller wiring, not by changing the replacement semantics).
func TestRegisterProviderSignalsRebooksLegacyBeforeReplacement(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	if err := Update(MetricsSource{
		ID: LegacyKeycloakMetricsID, Provider: capability.ProviderKeycloak,
		Class: SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: "alpha", Network: "baseharbor-alpha-dev",
		Target: "keycloak:9000", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	registrationID := "keycloak:bh-target-bravo-dev"
	if err := RegisterProviderSignals(ProviderSignalRegistration{
		ID:               registrationID,
		Descriptor:       capability.KeycloakIntegration,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "bravo",
		Enabled:          map[SignalKind]bool{SignalMetrics: true},
		Signals: map[string]ProviderSignalRuntime{"keycloak-metrics": {
			Network: "baseharbor-bravo-dev", Target: "keycloak:9000",
		}},
		Legacy: &LegacyReconciliation{
			Source:            LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}},
			Project:           "bh-target-bravo-identity-dev",
			CallerApplication: "bravo",
			OwnerProjects:     []map[string]string{{"alpha": "bh-target-alpha-identity-dev"}},
		},
	}); err != nil {
		t.Fatalf("register with legacy reconciliation: %v", err)
	}
	migratedID := "keycloak:bh-target-alpha-dev:" + LegacyKeycloakMetricsID
	sources, err := ListAll()
	if err != nil {
		t.Fatal(err)
	}
	var legacyRecord, newRecord *SignalSource
	for i := range sources {
		switch sources[i].ID {
		case migratedID:
			legacyRecord = &sources[i]
		case registrationID:
			newRecord = &sources[i]
		}
	}
	if legacyRecord == nil {
		t.Fatalf("the re-booked record is gone after the same-mutation replacement: %#v", sources)
	}
	if newRecord == nil {
		t.Fatal("the caller's registration is missing")
	}
	if legacyRecord.OwnerApplication != "alpha" || newRecord.OwnerApplication != "bravo" {
		t.Fatalf("owners were not kept verbatim: legacy=%q new=%q", legacyRecord.OwnerApplication, newRecord.OwnerApplication)
	}
}

// TestMigrateScopedNeverCollidesWithCallersRegistration proves the disjointness
// the review demands: the derived identity "<provider>:<project>:<legacy-id>"
// can never equal a caller's "<provider>:<project>" registration identity, so
// the (ID, provider) replacement inside one registration mutation cannot select
// the migrated record.
func TestMigrateScopedNeverCollidesWithCallersRegistration(t *testing.T) {
	legacy := LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}}
	derived := legacy.ScopedID(capability.ProviderKeycloak, "bh-target-alpha-dev")
	registration := "keycloak:bh-target-bravo-dev"
	if derived == registration {
		t.Fatalf("derived identity %q collapses onto the caller's registration identity", derived)
	}
	if len(strings.Split(derived, ":")) != 3 || len(strings.Split(registration, ":")) != 2 {
		t.Fatalf("grammar drift: scoped=%q registration=%q", derived, registration)
	}
}
