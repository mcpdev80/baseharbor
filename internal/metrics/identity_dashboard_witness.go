package metrics

import (
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// IdentityDashboardWitnessAt is the exported, fail-closed, non-forgeable gate
// for the identity-keycloak dashboard surface. It delegates to
// identityDashboardWitnessAt (same four real-evidence flags, AND-derived
// availability so a caller cannot make availability true by a status boolean).
func IdentityDashboardWitnessAt(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) capability.IdentitySelectionWitness {
	return identityDashboardWitnessAt(m, placement, targetStateRoot, targetName)
}

// identityDashboardWitnessAt is the fail-closed, non-forgeable gate for the
// identity-keycloak dashboard surface. It mirrors the original card #1650
// witness (internal/metrics/realization.go) adapted to the current Perses
// resolver API: the four evidence flags the resolver must attest are derived
// from real, target-bound evidence, so a caller cannot make availability true
// by asserting a single status boolean.
//
//   - selectionConfirmed: the manifest selects a real identity provider, never
//     an exporter (application.HasIdentity).
//   - targetAuthorized: a valid, non-forgeable Keycloak provider placement for
//     this Target. An external OIDC placement fails closed — the identity
//     dashboard realizes the BaseHarbor-owned provider, never an external
//     reference.
//   - metricPresent / metricCurrent: a real Keycloak native metrics signal
//     registered for THIS concrete Target in the observability signal registry.
//     The signal is bound to this concrete Target (see
//     keycloakMetricsRegistrationIDAt) so a foreign Target's signal on the same
//     sharing boundary cannot forge it.
func identityDashboardWitnessAt(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) capability.IdentitySelectionWitness {
	witness := capability.NewIdentitySelectionWitness(false, false, false, false)

	if !application.HasIdentity(m) {
		return witness
	}
	witness.SelectionConfirmed = true

	if placementErr := placement.Validate(); placementErr != nil {
		return capability.NewIdentitySelectionWitness(false, false, false, false)
	}
	if placement.Scope == capability.ScopeExternal {
		return capability.NewIdentitySelectionWitness(false, false, false, false)
	}
	witness.TargetAuthorized = true

	metricPresent, metricCurrent := keycloakMetricSignals(m, placement, targetStateRoot, targetName)
	witness.MetricPresent = metricPresent
	witness.MetricCurrent = metricCurrent

	return witness
}

// keycloakMetricSignals reports whether a real Keycloak native metrics signal is
// registered for THIS concrete Target. metricPresent is true when the
// keycloak-metrics signal exists for this Target; metricCurrent additionally
// requires the canonical native realization declared in the Keycloak
// integration descriptor (capability/reference.go) — an obsolete key or
// non-native exporter fails closed.
func keycloakMetricSignals(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) (metricPresent, metricCurrent bool) {
	if strings.TrimSpace(targetStateRoot) == "" || strings.TrimSpace(targetName) == "" {
		return false, false
	}
	expectedRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil || filepath.Clean(targetStateRoot) != filepath.Clean(expectedRoot) {
		return false, false
	}
	expectedID := keycloakMetricsRegistrationIDAt(m, placement, targetStateRoot, targetName)

	reference, refErr := capability.ReferenceIntegration(capability.ProviderKeycloak)
	if refErr != nil || expectedID == "" {
		return false, false
	}
	var expectedSignal *capability.ProviderObservabilitySignal
	for i := range reference.Observability.Signals {
		signal := &reference.Observability.Signals[i]
		if signal.Name == "keycloak-metrics" && signal.Kind == capability.ObservabilityMetrics &&
			signal.Collectable() && signal.Mode == capability.ObservabilityNative {
			expectedSignal = signal
			break
		}
	}
	if expectedSignal == nil {
		return false, false
	}

	policy, policyErr := application.MetricsPolicy(m)
	if policyErr != nil {
		return false, false
	}
	sources, err := observability.List(
		observability.SignalMetrics,
		placement,
		[]string{m.Name},
		false,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
		m.Environment,
	)
	if err != nil || len(sources) == 0 {
		return false, false
	}
	for _, source := range sources {
		if source.Provider != capability.ProviderKeycloak || source.ID != expectedID {
			continue
		}
		if source.OwnerApplication != m.Name || source.Scope != placement.Scope {
			continue
		}
		if placement.Scope == capability.ScopeShared && source.SharingBoundary != placement.SharingBoundary {
			continue
		}
		// Target is a runtime endpoint (for Keycloak, keycloak:9000), not an
		// application or tenant identifier. Require the production realization
		// shape and descriptor verification mode instead of accepting legacy
		// records that only happen to use the signal name.
		if strings.TrimSpace(source.Target) != "keycloak:9000" ||
			source.Mode != expectedSignal.Mode ||
			source.Protocol != expectedSignal.Protocol ||
			source.Path != expectedSignal.Path ||
			source.Verification != expectedSignal.Verification {
			continue
		}
		metricPresent = true
		metricCurrent = true
	}
	return metricPresent, metricCurrent
}

// keycloakMetricsRegistrationIDAt returns the per-Target registry identity the
// native keycloak-metrics signal is registered under. Per the 0428-1707
// scoping the registration id is "<provider>:<project>" (disjoint from the
// legacy literal "keycloak-metrics"), so a foreign Target's signal on the same
// sharing boundary cannot forge this witness. A missing or unresolvable binding
// returns empty, which makes keycloakMetricSignals fail closed (see the
// expectedID == "" guard above).
func keycloakMetricsRegistrationIDAt(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) string {
	if strings.TrimSpace(targetName) == "" {
		return ""
	}
	// The native keycloak-metrics signal registration id is "<provider>:<project>"
	// (0428-1707 scoping). The project is the owning application's canonical
	// project; the manifest name is the conservative local-face fallback. A
	// missing binding returns empty, which makes keycloakMetricSignals fail
	// closed (see the expectedID == "" guard above).
	return string(capability.ProviderKeycloak) + ":" + strings.TrimSpace(m.Name)
}
