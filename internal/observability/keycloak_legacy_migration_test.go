package observability

import (
	"os"
	"path/filepath"
	"testing"

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
	// No legacy literal exists at all: the change-aware path must not create
	// the registry file.
	legacy := LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}}
	migrated, err := legacy.MigrateScoped("bh-target-bravo-identity-dev")
	if err != nil {
		t.Fatalf("MigrateScoped on empty state: %v", err)
	}
	if migrated {
		t.Fatal("nothing exists, so nothing may report as migrated")
	}
	path := filepath.Join(dir, "observability", "signals.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a no-op migration must not create the registry file (stat err=%v)", err)
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
}

// TestMigrateScopedNeverCollidesWithCallersRegistration proves the disjointness
// the review demands: the derived identity "<provider>:<project>:<legacy-id>"
// can never equal a caller's "<provider>:<project>" registration identity, so
// the (ID, provider) replacement inside one registration mutation cannot select
// the migrated record.
func TestMigrateScopedNeverCollidesWithCallersRegistration(t *testing.T) {
	legacy := LegacySource{ID: LegacyKeycloakMetricsID, Providers: []capability.ProviderKind{capability.ProviderKeycloak}}
	derived := legacy.ScopedID(capability.ProviderKeycloak, "bh-target-alpha-dev")
	registration := "keycloak:bh-target-alpha"
	if derived == registration {
		t.Fatalf("derived identity %q collapses onto the caller's registration identity", derived)
	}
	if len(filepath.SplitList(derived)) != 3 || len(filepath.SplitList(registration)) != 2 {
		t.Fatalf("grammar drift: scoped=%q registration=%q", derived, registration)
	}
}
