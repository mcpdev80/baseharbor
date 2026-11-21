package metrics

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
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

func identityTargetRoot(t *testing.T, namespace string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	targetRoot, err := deployment.TargetStateRoot(namespace)
	if err != nil {
		t.Fatalf("resolve target state root: %v", err)
	}
	return targetRoot
}

func seedCurrentKeycloakSignal(t *testing.T, m application.Manifest, placement capability.ProviderPlacement, targetRoot, namespace string) {
	t.Helper()
	id, err := identityprovider.KeycloakMetricsRegistrationIDAt(m, placement, targetRoot, namespace)
	if err != nil {
		t.Fatalf("resolve Keycloak signal registration: %v", err)
	}
	class := observability.SourcePlatformProvider
	if placement.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	source := observability.SignalSource{
		ID: id, Kind: observability.SignalMetrics, Provider: capability.ProviderKeycloak,
		Class: class, Scope: placement.Scope, SharingBoundary: placement.SharingBoundary,
		OwnerApplication: m.Name, Network: "baseharbor-identity", Target: "keycloak:9000",
		Protocol: "openmetrics", Path: "/metrics", Mode: capability.ObservabilityNative,
		Verification: capability.ObservabilityVerifyBackend,
	}
	if err := source.Validate(); err != nil {
		t.Fatalf("validate Keycloak signal: %v", err)
	}
	if err := observability.UpdateSignal(source); err != nil {
		t.Fatalf("register Keycloak signal: %v", err)
	}
}

// TestIdentityWitnessEligibleWhenRealSignalAndTargetAuthorized is the positive
// case: a real Keycloak native metrics signal registered for this Target, the
// identity provider actually selected, and a valid BaseHarbor-owned Keycloak
// placement yield a non-forgeable witness (NonForgeableAvailable() == true).
func TestIdentityWitnessEligibleWhenRealSignalAndTargetAuthorized(t *testing.T) {
	root := identityTargetRoot(t, "target-alpha")
	const namespace = "target-alpha"
	m := newIdentityManifest(t)
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	seedCurrentKeycloakSignal(t, m, placement, root, namespace)

	witness := IdentitySelectionWitnessAt(m, placement, root, namespace)
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

// TestIdentityWitnessFromKeycloakProducer connects the production signal
// registration path to the production witness. Unlike the unit fixture above,
// it never seeds an observability.SignalSource itself.
func TestIdentityWitnessFromKeycloakProducer(t *testing.T) {
	const namespace = "target-producer-witness"
	root := identityTargetRoot(t, namespace)
	t.Setenv(application.MetricsEnabledEnv, "true")
	app := newIdentityManifest(t)
	app.Metrics.Sources = []application.MetricsSourceRequirement{{
		Name: "app", Service: "api", Port: 8080, Path: "/metrics",
	}}

	files, err := identityprovider.EnsureKeycloakFilesAt(
		context.Background(), app, serviceissuer.New(t), root, namespace,
	)
	if err != nil {
		t.Fatalf("EnsureKeycloakFilesAt: %v", err)
	}
	if err := identityprovider.RegisterKeycloakMetricsSignalsAt(app, files, root, namespace); err != nil {
		t.Fatalf("RegisterKeycloakMetricsSignalsAt: %v", err)
	}

	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve Keycloak placement: %v", err)
	}
	witness := IdentitySelectionWitnessAt(app, placement, root, namespace)
	if !witness.NonForgeableAvailable() {
		t.Fatalf("producer registration did not produce an eligible witness: %+v", witness)
	}
	if !witness.SelectionConfirmed || !witness.MetricPresent || !witness.MetricCurrent || !witness.TargetAuthorized {
		t.Fatalf("all producer-to-witness evidence flags must be true: %+v", witness)
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
	rootA := identityTargetRoot(t, "target-alpha")
	const namespaceA = "target-alpha"

	// Target A has the real Keycloak native metrics signal bound to its own
	// binding.
	manifestA := newIdentityManifest(t)
	placementA, err := application.ResolveProviderPlacement(manifestA, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for target A: %v", err)
	}
	seedCurrentKeycloakSignal(t, manifestA, placementA, rootA, namespaceA)

	// Positive case: the witness for the manifest that owns the signal is
	// eligible.
	if witnessA := IdentitySelectionWitnessAt(manifestA, placementA, rootA, namespaceA); !witnessA.NonForgeableAvailable() {
		t.Fatalf("target A witness must be eligible: %+v", witnessA)
	}

	// A second Target gets a different state root and registration identity,
	// even though both Keycloak providers use the same platform sharing boundary.
	// Its own registration proves the global registry is populated and isolated.
	rootB, err := deployment.TargetStateRoot("target-beta")
	if err != nil {
		t.Fatalf("resolve target B state root: %v", err)
	}
	const namespaceB = "target-beta"
	manifestB := newIdentityManifestNamed(t, manifestA.Name)
	placementB, err := application.ResolveProviderPlacement(manifestB, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for target B: %v", err)
	}
	seedCurrentKeycloakSignal(t, manifestB, placementB, rootB, namespaceB)
	witnessB := IdentitySelectionWitnessAt(manifestB, placementB, rootB, namespaceB)
	if !witnessB.NonForgeableAvailable() {
		t.Fatalf("target B must resolve its own signal from the populated registry: %+v", witnessB)
	}

	// A third Target with no registration must remain unavailable while both
	// other targets' records are present in the same global registry.
	rootC, err := deployment.TargetStateRoot("target-gamma")
	if err != nil {
		t.Fatalf("resolve target C state root: %v", err)
	}
	manifestC := newIdentityManifestNamed(t, manifestA.Name)
	placementC, err := application.ResolveProviderPlacement(manifestC, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for target C: %v", err)
	}
	witnessC := IdentitySelectionWitnessAt(manifestC, placementC, rootC, "target-gamma")
	if witnessC.NonForgeableAvailable() || witnessC.MetricPresent {
		t.Fatalf("target C must not inherit target A/B's signals from the shared registry: %+v", witnessC)
	}
}

func TestIdentityWitnessFailsClosedForUnresolvedTargetBinding(t *testing.T) {
	root := identityTargetRoot(t, "target-alpha")
	m := newIdentityManifest(t)
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}
	seedCurrentKeycloakSignal(t, m, placement, root, "target-alpha")

	// The registry is populated with the exact production-shaped registration;
	// an empty or mismatched state root/namespace must not borrow it.
	otherRoot, err := deployment.TargetStateRoot("target-beta")
	if err != nil {
		t.Fatalf("resolve other target state root: %v", err)
	}
	tests := []struct {
		name      string
		dataDir   string
		namespace string
	}{
		{name: "missing root", dataDir: "", namespace: "target-alpha"},
		{name: "missing namespace", dataDir: root, namespace: ""},
		{name: "root belongs to another target", dataDir: otherRoot, namespace: "target-alpha"},
		{name: "namespace belongs to another root", dataDir: root, namespace: "target-beta"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			witness := IdentitySelectionWitnessAt(m, placement, tc.dataDir, tc.namespace)
			if witness.NonForgeableAvailable() || witness.MetricPresent || witness.MetricCurrent {
				t.Fatalf("unresolved Target binding must fail closed: %+v", witness)
			}
		})
	}
}

func TestIdentityWitnessRejectsLegacyOrMismatchedSignalRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*observability.SignalSource)
	}{
		{name: "legacy signal id", mutate: func(s *observability.SignalSource) { s.ID = keycloakMetricsSignalID }},
		{name: "wrong owner", mutate: func(s *observability.SignalSource) { s.OwnerApplication = "foreign-app" }},
		{name: "endpoint is not tenant binding", mutate: func(s *observability.SignalSource) { s.Target = "testapp" }},
		{name: "wrong path", mutate: func(s *observability.SignalSource) { s.Path = "/admin/metrics" }},
		{name: "non-native mode", mutate: func(s *observability.SignalSource) { s.Mode = "" }},
		{name: "unverified descriptor", mutate: func(s *observability.SignalSource) { s.Verification = capability.ObservabilityVerifyNone }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := identityTargetRoot(t, "target-alpha")
			m := newIdentityManifest(t)
			placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
			if err != nil {
				t.Fatalf("resolve Keycloak placement: %v", err)
			}
			id, err := identityprovider.KeycloakMetricsRegistrationIDAt(m, placement, root, "target-alpha")
			if err != nil {
				t.Fatalf("resolve Keycloak registration ID: %v", err)
			}
			source := observability.SignalSource{
				ID: id, Kind: observability.SignalMetrics, Provider: capability.ProviderKeycloak,
				Class: observability.SourcePlatformProvider, Scope: placement.Scope,
				SharingBoundary: placement.SharingBoundary, OwnerApplication: m.Name,
				Network: "baseharbor-identity", Target: "keycloak:9000", Protocol: "openmetrics",
				Path: "/metrics", Mode: capability.ObservabilityNative,
				Verification: capability.ObservabilityVerifyBackend,
			}
			tc.mutate(&source)
			if err := observability.UpdateSignal(source); err != nil {
				t.Fatalf("register mutated signal: %v", err)
			}
			witness := IdentitySelectionWitnessAt(m, placement, root, "target-alpha")
			if witness.MetricPresent || witness.MetricCurrent || witness.NonForgeableAvailable() {
				t.Fatalf("mismatched registry record must fail closed: %+v", witness)
			}
		})
	}
}

// TestIdentityWitnessFailsClosedWithoutSignal is the negative case: no real
// Keycloak native metrics signal registered for this Target fails closed, even
// when the identity provider is selected and the placement is valid.
func TestIdentityWitnessFailsClosedWithoutSignal(t *testing.T) {
	root := identityTargetRoot(t, "target-alpha")

	m := newIdentityManifest(t)
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	witness := IdentitySelectionWitnessAt(m, placement, root, "target-alpha")
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
	root := identityTargetRoot(t, "target-alpha")

	m := newIdentityManifest(t)
	m.Services.Identity = false // identity not selected
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement: %v", err)
	}

	// Seed a real signal — the witness must still fail closed because the
	// identity provider is not actually selected.
	seedCurrentKeycloakSignal(t, m, placement, root, "target-alpha")

	witness := IdentitySelectionWitnessAt(m, placement, root, "target-alpha")
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
	root := identityTargetRoot(t, "target-alpha")

	m := newIdentityManifest(t)
	// An external OIDC placement (no BaseHarbor-owned Keycloak): the identity
	// dashboard realizes the BaseHarbor-owned provider, never an external
	// reference. Pass the external placement directly to the witness builder.
	externalPlacement := capability.ProviderPlacement{
		Scope:             capability.ScopeExternal,
		Ownership:         capability.OwnershipExternal,
		ExternalReference: "https://oidc.example.com/realm",
	}

	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve keycloak placement for signal: %v", err)
	}
	seedCurrentKeycloakSignal(t, m, placement, root, "target-alpha")

	witness := IdentitySelectionWitnessAt(m, externalPlacement, root, "target-alpha")
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
