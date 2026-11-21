package metrics

import (
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// keycloakMetricsSignalID is the native, openmetrics metrics signal the Keycloak
// identity provider exposes (per the Keycloak integration descriptor in
// capability/reference.go: keycloak-metrics, Mode native, path /metrics). This
// is the only real signal the identity dashboard's MetricPresent/MetricCurrent
// flags derive from — never a caller-supplied guess.
const keycloakMetricsSignalID = "keycloak-metrics"

// IdentitySelectionWitnessAt builds the authoritative, non-forgeable
// identity-selection witness for ONE manifest/Target from real production
// primitives only (never a caller status):
//
//   - SelectionConfirmed   = the identity provider is actually selected
//     (application.HasIdentity) — never an assumed exporter.
//   - MetricPresent        = a real Keycloak native metrics signal exists for
//     this Target in the observability signal registry (reference.go declares
//     it; the registry realizes it).
//   - MetricCurrent        = that signal is a native openmetrics signal (an
//     obsolete-key or non-native exporter fails closed).
//   - TargetAuthorized     = the Keycloak provider placement is a valid,
//     non-forgeable BaseHarbor-owned placement for this Target (never external).
//
// Any missing flag fails closed to an unavailable witness (the zero value of
// capability.IdentitySelectionWitness). This is the single, validated witness
// every production gate must use; it cannot be forged by supplying a status
// boolean.
//
// dataDir/namespace must identify the canonical Target state root. The
// registry is shared, so the signal is additionally matched against the
// registration ID derived from that target's Keycloak state, its owner, scope,
// and sharing boundary. Endpoint-valued SignalSource.Target is never treated as
// a tenant ID. Missing or ambiguous bindings fail closed.
func IdentitySelectionWitnessAt(m application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) capability.IdentitySelectionWitness {
	witness := capability.NewIdentitySelectionWitness(false, false, false, false)

	// SelectionConfirmed: real manifest identity selection, never an exporter.
	if !application.HasIdentity(m) {
		return witness
	}
	witness.SelectionConfirmed = true

	// TargetAuthorized: a valid, non-forgeable Keycloak provider placement for
	// this Target. The caller passes ResolveProviderPlacement's result; an
	// external OIDC placement (no BaseHarbor-owned Keycloak) fails closed — the
	// identity dashboard realizes the BaseHarbor-owned provider, never an
	// external reference.
	if placementErr := placement.Validate(); placementErr != nil {
		return capability.NewIdentitySelectionWitness(false, false, false, false)
	}
	if placement.Scope == capability.ScopeExternal {
		return capability.NewIdentitySelectionWitness(false, false, false, false)
	}
	witness.TargetAuthorized = true

	// MetricPresent/MetricCurrent: a real Keycloak native metrics signal
	// registered for THIS Target in the observability signal registry. The signal
	// is bound to this concrete Target (see keycloakMetricSignals) so a foreign
	// Target's signal on the same sharing boundary cannot forge it.
	metricPresent, metricCurrent := keycloakMetricSignals(m, placement, dataDir, namespace)
	witness.MetricPresent = metricPresent
	witness.MetricCurrent = metricCurrent

	// Available is derived (AND) inside NewIdentitySelectionWitness — a caller
	// can never make it true by omission; the zero value stays unavailable.
	return witness
}

// keycloakMetricSignals reports whether a real Keycloak native metrics signal is
// registered for THIS concrete Target. metricPresent is true when the
// keycloak-metrics signal exists for this Target; metricCurrent additionally
// requires the native realization declared in the Keycloak integration
// descriptor (reference.go) — an obsolete-key or non-native exporter fails
// closed. A native keycloak signal of ANOTHER Target on the same sharing
// boundary is excluded (fail-closed): the signal must resolve to this Target's
// binding, never a foreign Target on the same platform boundary.
func keycloakMetricSignals(m application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) (metricPresent, metricCurrent bool) {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(namespace) == "" {
		return false, false
	}
	expectedRoot, err := deployment.TargetStateRoot(namespace)
	if err != nil || filepath.Clean(dataDir) != filepath.Clean(expectedRoot) {
		return false, false
	}
	expectedID, err := identityprovider.KeycloakMetricsRegistrationIDAt(m, placement, dataDir, namespace)
	if err != nil {
		return false, false
	}
	// The canonical, current realization of the keycloak-metrics signal is the
	// native openmetrics signal declared in the Keycloak integration descriptor
	// (capability/reference.go). This is the "current, accepted Perses release"
	// the MetricCurrent flag gates on — never an assumed exporter.
	reference, refErr := capability.ReferenceIntegration(capability.ProviderKeycloak)
	if refErr != nil {
		return false, false
	}
	var expectedSignal *capability.ProviderObservabilitySignal
	for i := range reference.Observability.Signals {
		signal := &reference.Observability.Signals[i]
		if signal.Name == keycloakMetricsSignalID && signal.Kind == capability.ObservabilityMetrics &&
			signal.Collectable() && signal.Mode == capability.ObservabilityNative {
			expectedSignal = signal
			break
		}
	}
	if expectedSignal == nil {
		return false, false
	}

	// The signal must also be realized for this concrete Target in the
	// observability signal registry (reference.go declares it; the registry
	// realizes it). A missing/unresolvable target binding fails closed.
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
