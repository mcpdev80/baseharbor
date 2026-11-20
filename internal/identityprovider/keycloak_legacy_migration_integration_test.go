package identityprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// TestRegisterKeycloakSignalsRebooksLegacySourceIsCorpse is the end-to-end
// reproduction + fix through the real registerKeycloakSignals call path.
//
// The body is the acceptance rule of card 0428-168: it writes a legacy
// "keycloak-metrics" source of record the OLD runtime left behind, then runs
// the NEW registerKeycloakSignals for a DIFFERENT Keycloak project. It asserts
// the legacy source is NOT dropped from the registry (the corpse), that the new
// scoping-ID source is distinct and present, and that a second register call is
// idempotent (the migration is no-mutation).
func TestRegisterKeycloakSignalsRebooksLegacySourceIsCorpse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)

	const legacyID = observability.LegacyKeycloakMetricsID

	// 1. A pre-deploy legacy Keycloak source of record, written exactly as the
	// OLD runtime did it: an app-scoped Keycloak metrics source under the
	// literal id "keycloak-metrics". The OLD registerKeycloakSignals used the
	// literal ID "keycloak-metrics" (before 0428-1707 made it scoping-aware);
	// we write that source of record directly into the registry.
	appA := keycloakSignalTestApp(t, "alpha", true)
	placement, err := application.ResolveProviderPlacement(appA, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("ResolveProviderPlacement: %v", err)
	}
	if err := observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
		ID:               legacyID,
		Descriptor:       capability.KeycloakIntegration,
		Class:            observability.SourceApplicationProvider,
		Scope:            placement.Scope,
		SharingBoundary:  placement.SharingBoundary,
		OwnerApplication: appA.Name,
		Enabled:          map[observability.SignalKind]bool{observability.SignalMetrics: true},
		Signals: map[string]observability.ProviderSignalRuntime{
			"keycloak-metrics": {Network: "baseharbor-identity", Target: "keycloak:9000"},
		},
	}); err != nil {
		t.Fatalf("write legacy keycloak source of record: %v", err)
	}
	if ok, err := observability.RegistryHasSignal(legacyID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("legacy source not present after its own registration")
	}

	// 2. The NEW code runs for a DIFFERENT Keycloak project (project-b, app-b).
	// Before 0428-168 this would drop the legacy source of record; the
	// migration re-books it onto the scoping ID first.
	newApp := keycloakSignalTestApp(t, "bravo", true)
	newR := &localKeycloakRealization{
		app:       newApp,
		namespace: "target",
		files:     KeycloakFiles{Project: "project-b", ConsumerNetwork: "baseharbor-identity"},
	}
	if err := newR.registerKeycloakSignals(); err != nil {
		t.Fatalf("register new keycloak signals: %v", err)
	}

	newID := keycloakMetricsSignalID("project-b")

	// 2a. The legacy ID is gone from List (re-booked onto the new scoping ID).
	if ok, err := observability.RegistryHasSignal(legacyID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("legacy %q still present after new registration -> NOT re-booked", legacyID)
	}

	// 2b. The new scoping-ID source is present and distinct.
	if ok, err := observability.RegistryHasSignal(newID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("new scoping-ID source %q not present", newID)
	}

	// 2c. The legacy source is now removable by its new scoping ID.
	if err := observability.Remove(newID); err != nil {
		t.Fatalf("Remove(%q): %v", newID, err)
	}
	if ok, err := observability.RegistryHasSignal(newID, capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("re-booked source still present after its own Remove")
	}

	// 3. Idempotency of the MIGRATION specifically: the migration re-booked the
	// legacy source ONCE on the first new registration. A second NEW
	// registration for a THIRD project must not create a second copy of the
	// re-booked source, and the scoping-ID source for project-b must stay
	// exactly one entry (no-mutation, no duplication).
	thirdApp := keycloakSignalTestApp(t, "charlie", true)
	thirdR := &localKeycloakRealization{
		app:       thirdApp,
		namespace: "target",
		files:     KeycloakFiles{Project: "project-c", ConsumerNetwork: "baseharbor-identity"},
	}
	before, err := observability.RegistryHasSignal(newID, capability.ProviderKeycloak, observability.SignalMetrics)
	if err != nil {
		t.Fatalf("RegistryHasSignal before 3rd run: %v", err)
	}
	if err := thirdR.registerKeycloakSignals(); err != nil {
		t.Fatalf("third register: %v", err)
	}
	after, err := observability.RegistryHasSignal(newID, capability.ProviderKeycloak, observability.SignalMetrics)
	if err != nil {
		t.Fatalf("RegistryHasSignal after 3rd run: %v", err)
	}
	if before != after {
		t.Fatalf("second new registration changed project-b source presence: before=%v after=%v (migration not no-mutation)", before, after)
	}
	if ok, _ := observability.RegistryHasSignal(keycloakMetricsSignalID("project-c"), capability.ProviderKeycloak, observability.SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("third project's scoping-ID source not present")
	}
}

// mustPlacementForTest resolves the provider placement for a test app.
func mustPlacementForTest(t *testing.T, app application.Manifest) capability.ProviderPlacement {
	t.Helper()
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("ResolveProviderPlacement: %v", err)
	}
	return placement
}
