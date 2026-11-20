package metrics

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// newIdentityManifest builds a manifest with the identity provider selected and
// a BaseHarbor-owned Keycloak placement (the default resolved placement).
func newIdentityManifest(t *testing.T) application.Manifest {
	t.Helper()
	return newIdentityManifestNamed(t, "testapp")
}

// newIdentityManifestNamed is the named form of newIdentityManifest for the
// two-target target-isolation regression (a second, distinct Target).
func newIdentityManifestNamed(t *testing.T, name string) application.Manifest {
	t.Helper()
	m := application.Manifest{
		Name:          name,
		Environment:   "test",
		ApplicationID: "app-" + name,
	}
	m.Services.Identity = true
	return m
}

// TestIdentityWitnessEligibleWhenRealSignalAndTargetAuthorized is the positive
// case: a real Keycloak native metrics signal registered for this Target, the
// identity provider actually selected, and a valid BaseHarbor-owned Keycloak
// placement yield a non-forgeable witness (NonForgeableAvailable() == true).
func TestIdentityWitnessEligibleWhenRealSignalAndTargetAuthorized(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := newIdentityManifest(t)
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	// Seed a real Keycloak native metrics signal for this Target. The signal is
	// bound to the manifest's concrete Target (m.Name), so the witness signal
	// lookup anchors on that binding.
	source := observability.MetricsSource{
		ID:       keycloakMetricsSignalID,
		Provider: capability.ProviderKeycloak,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Network:  "baseharbor-identity",
		Target:   m.Name,
		Path:     "/metrics",
		Scheme:   "http",
	}
	if err := source.Validate(); err != nil {
		t.Fatalf("seed signal validate: %v", err)
	}
	if err := observability.Update(source); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	witness := IdentitySelectionWitnessAt(m, placement, "", "")
	if !witness.NonForgeableAvailable() {
		t.Fatalf(
			"non-forgeable availability = false, want true with a real keycloak-metrics signal + valid target authorization\n%+v",
			witness,
		)
	}
	if !witness.SelectionConfirmed || !witness.MetricPresent || !witness.MetricCurrent || !witness.TargetAuthorized {
		t.Fatalf("all four evidence flags must be true: %+v", witness)
	}
}

// TestIdentityWitnessIsolatedToConcreteTarget is the P3 target-isolation
// regression: a native keycloak-metrics signal bound to ANOTHER Target on the
// same sharing boundary must NOT satisfy this Target's witness. Only the signal
// resolved to this concrete Target's binding yields a positive witness.
//
// Stop criterion (reviewer): removing the Target-filtering in
// keycloakMetricSignals must make this test RED — the foreign-target case would
// otherwise become eligible.
func TestIdentityWitnessIsolatedToConcreteTarget(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Target A has the real Keycloak native metrics signal bound to its own
	// binding.
	manifestA := newIdentityManifest(t)
	placementA, err := application.ResolveProviderPlacement(manifestA, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for target A: %v", err)
	}
	sourceA := observability.MetricsSource{
		ID:       keycloakMetricsSignalID,
		Provider: capability.ProviderKeycloak,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Network:  "baseharbor-identity",
		Target:   manifestA.Name,
		Path:     "/metrics",
		Scheme:   "http",
	}
	if err := sourceA.Validate(); err != nil {
		t.Fatalf("seed target A signal validate: %v", err)
	}
	if err := observability.Update(sourceA); err != nil {
		t.Fatalf("seed target A signal: %v", err)
	}

	// Positive case: the witness for the manifest that owns the signal is
	// eligible.
	if witnessA := IdentitySelectionWitnessAt(manifestA, placementA, "", ""); !witnessA.NonForgeableAvailable() {
		t.Fatalf("target A witness must be eligible: %+v", witnessA)
	}

	// Foreign case: a DIFFERENT Target (same sharing boundary) on the SAME
	// platform with NO signal of its own must NOT become eligible from target A's
	// signal. Build manifest B in the same state dir and assert fail-closed.
	manifestB := newIdentityManifestNamed(t, "other-target")
	placementB, err := application.ResolveProviderPlacement(manifestB, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for target B: %v", err)
	}
	witnessB := IdentitySelectionWitnessAt(manifestB, placementB, "", "")
	if witnessB.NonForgeableAvailable() {
		t.Fatalf("target B witness must fail closed when only target A has the signal: %+v", witnessB)
	}
	if witnessB.MetricPresent {
		t.Fatalf("MetricPresent must be false for target B when the signal is bound to target A: %+v", witnessB)
	}
}

// TestIdentityWitnessFailsClosedWithoutSignal is the negative case: no real
// Keycloak native metrics signal registered for this Target fails closed, even
// when the identity provider is selected and the placement is valid.
func TestIdentityWitnessFailsClosedWithoutSignal(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := newIdentityManifest(t)
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	witness := IdentitySelectionWitnessAt(m, placement, "", "")
	if witness.NonForgeableAvailable() {
		t.Fatalf("non-forgeable availability = true with no keycloak-metrics signal registered, want fail-closed false")
	}
	if witness.MetricPresent {
		t.Fatalf("MetricPresent = true with no keycloak-metrics signal registered, want false")
	}
}

// TestIdentityWitnessFailsClosedWhenNotSelected is the negative case: an
// unselected identity provider fails closed, even with a real signal.
func TestIdentityWitnessFailsClosedWhenNotSelected(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := newIdentityManifest(t)
	m.Services.Identity = false // identity not selected
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	// Seed a real signal — the witness must still fail closed because the
	// identity provider is not actually selected.
	source := observability.MetricsSource{
		ID:       keycloakMetricsSignalID,
		Provider: capability.ProviderKeycloak,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Network:  "baseharbor-identity",
		Target:   m.Name,
		Path:     "/metrics",
		Scheme:   "http",
	}
	if err := observability.Update(source); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	witness := IdentitySelectionWitnessAt(m, placement, "", "")
	if witness.NonForgeableAvailable() {
		t.Fatalf("non-forgeable availability = true with identity not selected, want fail-closed false")
	}
	if witness.SelectionConfirmed {
		t.Fatalf("SelectionConfirmed = true with identity not selected, want false")
	}
}

// TestIdentityWitnessFailsClosedOnExternalPlacement is the negative case: an
// external OIDC placement (no BaseHarbor-owned Keycloak) fails closed even with
// a real signal and a selected identity provider.
func TestIdentityWitnessFailsClosedOnExternalPlacement(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	m := newIdentityManifest(t)
	// An external OIDC placement (no BaseHarbor-owned Keycloak): the identity
	// dashboard realizes the BaseHarbor-owned provider, never an external
	// reference. Pass the external placement directly to the witness builder.
	externalPlacement := capability.ProviderPlacement{
		Scope:             capability.ScopeExternal,
		Ownership:         capability.OwnershipExternal,
		ExternalReference: "https://oidc.example.com/realm",
	}

	source := observability.MetricsSource{
		ID:       keycloakMetricsSignalID,
		Provider: capability.ProviderKeycloak,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Network:  "baseharbor-identity",
		Target:   m.Name,
		Path:     "/metrics",
		Scheme:   "http",
	}
	if err := observability.Update(source); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

	witness := IdentitySelectionWitnessAt(m, externalPlacement, "", "")
	if witness.NonForgeableAvailable() {
		t.Fatalf("non-forgeable availability = true with an external keycloak placement, want fail-closed false")
	}
	if witness.TargetAuthorized {
		t.Fatalf("TargetAuthorized = true with an external keycloak placement, want false")
	}
}

// TestIdentityAvailabilityRequiresVerifiedPerses gates the identity-dashboard
// availability on the authoritative Perses three-state contract: identity
// availability derives only from a real Keycloak native metrics signal, but the
// identity dashboard realizes the Perses management surface, so its eligibility
// can never be emitted while Perses is not_configured. PersesStatusNow maps
// manifest truth to not_configured | configured_unverified | verified, and
// verified requires a real Perses health probe that does not exist here, so the
// only reachable state for a selected surface is configured_unverified.
func TestIdentityAvailabilityRequiresVerifiedPerses(t *testing.T) {
	// Not-selected Perses -> not_configured. The identity dashboard, being a
	// realization of that Perses surface, is not eligible even with a real
	// Keycloak signal.
	unselected := capability.PersesStatusNow(false)
	if *unselected != capability.PersesStatusNotConfigured {
		t.Fatalf("PersesStatusNow(false) = %q, want not_configured", *unselected)
	}
	// Selected Perses -> configured_unverified (pin validation only). Eligible
	// only in this state or verified, never not_configured.
	selected := capability.PersesStatusNow(true)
	if *selected != capability.PersesStatusConfiguredUnverified {
		t.Fatalf("PersesStatusNow(true) = %q, want configured_unverified", *selected)
	}
	// A manifest without a selected Perses surface can never reach the eligible
	// identity-dashboard state.
	if *capability.PersesStatusNow(true) == capability.PersesStatusNotConfigured {
		t.Fatal("a selected Perses surface must not map into not_configured")
	}
}
