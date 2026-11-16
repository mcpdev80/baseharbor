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
	m := application.Manifest{
		Name:          "testapp",
		Environment:   "test",
		ApplicationID: "app-test-0001",
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

	// Seed a real Keycloak native metrics signal for this Target.
	source := observability.MetricsSource{
		ID:       keycloakMetricsSignalID,
		Provider: capability.ProviderKeycloak,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Network:  "baseharbor-identity",
		Target:   "keycloak:9000",
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
		Target:   "keycloak:9000",
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
		Target:   "keycloak:9000",
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
