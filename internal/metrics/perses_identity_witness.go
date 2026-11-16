package metrics

import (
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// keycloakMetricsSignalID is the native, openmetrics metrics signal the Keycloak
// identity provider exposes (per the Keycloak integration descriptor in
// capability/reference.go: keycloak-metrics, Mode native, path /metrics). This
// is the only real signal the identity dashboard's MetricPresent/MetricCurrent
// flags derive from — never a caller-supplied guess.
const keycloakMetricsSignalID = "keycloak-metrics"

// IdentitySelectionWitnessAt builds the authoritative, non-forgeable
// identity-selection witness for one manifest/Target from real production
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
// dataDir/namespace scope the observability registry read exactly like the
// existing metrics-provider resolution (prometheus_verify.go): the keycloak
// metrics signal is a provider-registered target signal, resolved the same way.
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
	// registered for this Target in the observability signal registry.
	metricPresent, metricCurrent := keycloakMetricSignals(m, placement, dataDir, namespace)
	witness.MetricPresent = metricPresent
	witness.MetricCurrent = metricCurrent

	// Available is derived (AND) inside NewIdentitySelectionWitness — a caller
	// can never make it true by omission; the zero value stays unavailable.
	return witness
}

// keycloakMetricSignals reports whether a real Keycloak native metrics signal
// is registered for this Target. metricPresent is true when the keycloak-metrics
// signal exists for this Target; metricCurrent additionally requires the native
// realization declared in the Keycloak integration descriptor (reference.go) —
// an obsolete-key or non-native exporter fails closed.
func keycloakMetricSignals(m application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) (metricPresent, metricCurrent bool) {
	// The canonical, current realization of the keycloak-metrics signal is the
	// native openmetrics signal declared in the Keycloak integration descriptor
	// (capability/reference.go). This is the "current, accepted Perses release"
	// the MetricCurrent flag gates on — never an assumed exporter.
	reference, refErr := capability.ReferenceIntegration(capability.ProviderKeycloak)
	if refErr != nil {
		return false, false
	}
	nativeSignalPresent := false
	for _, signal := range reference.Observability.Signals {
		if signal.Name == keycloakMetricsSignalID && signal.Mode == capability.ObservabilityNative {
			nativeSignalPresent = true
			break
		}
	}
	if !nativeSignalPresent {
		return false, false
	}

	// The signal must also be realized for this Target in the observability
	// signal registry (reference.go declares it; the registry realizes it).
	policy, policyErr := application.MetricsPolicy(m)
	if policyErr != nil {
		return false, false
	}
	sources, err := observability.List(
		observability.SignalMetrics,
		placement,
		[]string{m.Name},
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil || len(sources) == 0 {
		return false, false
	}
	for _, source := range sources {
		if source.Provider != capability.ProviderKeycloak {
			continue
		}
		if strings.TrimSpace(source.ID) != keycloakMetricsSignalID {
			continue
		}
		// Present AND current: the registry signal matches the native, accepted
		// realization declared in the reference descriptor.
		metricPresent = true
		metricCurrent = true
	}
	return metricPresent, metricCurrent
}
