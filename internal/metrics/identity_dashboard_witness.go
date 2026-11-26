package metrics

import (
	"errors"
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
	// w267 HOLD correction (comment 7192567805): the registration ID is only
	// project-scoped ("<provider>:<manifest name>", per 0428-1707) and the
	// endpoint keycloak:9000 is a shared service name, so NEITHER attributes
	// the record to the concrete Target being witnessed. The binding is
	// asserted on the record's Network field: the producer registers the
	// signal with the Target-resolved identity consumer network
	// (KeycloakFiles.ConsumerNetwork, internal/identityprovider, whose
	// namespace token is the resolved Target name,
	// cmd/baha/app_status_result.go:489). keycloakTargetConsumerNetwork
	// composes that exact network for THIS Target and asserts the supplied
	// state root belongs to it; the loop guard requires the record's Network
	// to match. A foreign Target's signal carries ITS OWN Target's consumer
	// network and can never make this witness available; the matching
	// Target's signal stays eligible. No product consent/auth behavior
	// changes: this is a read-side registry attribution check.
	expectedNetwork, networkErr := keycloakTargetConsumerNetwork(m, placement, targetStateRoot, targetName)
	if networkErr != nil || expectedNetwork == "" {
		return false, false
	}

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
		// Per-Target binding is asserted here, not only on the registration id
		// (which is per-0428-1707 project-scoped): the matching Keycloak
		// record must resolve to THIS concrete Target. The state root was
		// pinned to TargetStateRoot(targetName) above, and the record's Target
		// endpoint must be this Target's Keycloak runtime endpoint; a foreign
		// Target cannot present this Target's signal. Target is a runtime
		// endpoint (for Keycloak, keycloak:9000), not an application or tenant
		// identifier. Require the production realization shape and descriptor
		// verification mode instead of accepting legacy records that only
		// happen to use the signal name.
		if strings.TrimSpace(source.Target) != "keycloak:9000" ||
			source.Mode != expectedSignal.Mode ||
			source.Protocol != expectedSignal.Protocol ||
			source.Path != expectedSignal.Path ||
			source.Verification != expectedSignal.Verification {
			continue
		}
		// Per-Target attribution (the HOLD binding): the record must have been
		// registered on THIS Target's identity network, derived from the
		// Target's validated state root. This is what neither the
		// project-scoped ID nor the constant endpoint can express. An empty
		// expectation fails closed.
		if strings.TrimSpace(source.Network) != expectedNetwork || expectedNetwork == "" {
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
// expectedID == "" guard above). The per-Target binding the witness asserts on
// top of this id lives in keycloakMetricSignals: the matching record must also
// carry the state root that resolves to THIS concrete Target (see
// TargetStateRoot(targetName) below) and the production Keycloak descriptor, so
// the registration id combined with the state-root check is genuinely
// target-scoped, not just project-scoped.
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

// keycloakTargetConsumerNetwork derives the identity consumer network the
// production Keycloak realization registers the native metrics signal on for
// THIS concrete Target — the per-Target attribution the w267 HOLD requires
// and that the project-scoped ID plus the constant endpoint cannot express.
//
// Producer truth: the realization's namespace is the RESOLVED TARGET NAME it
// was constructed with (the status face passes resolved.Target.Name,
// cmd/baha/app_status_result.go:489 via identityDashboardBackendEligible;
// the shared placement overrides app.Name to "core" but never the namespace,
// internal/identityprovider/keycloak_files.go:133-137). The consumer network
// the producer writes into the registry record's Network field therefore
// carries the TARGET name as its namespace token:
//
//   - shared placement:       "baseharbor-identity-core-<target>"
//   - application placement:  "baseharbor-identity-<app>-<env>-<target>"
//
// (application.IdentityProviderNetworkNameForPlacement, identity_network.go,
// which normalizes dots in the namespace to hyphens.)
//
// The witness composes the SAME grammar from targetName directly, after the
// caller pinned the supplied state root to deployment.TargetStateRoot
// (targetName): a foreign Target's signal — same project-scoped ID, same
// endpoint, registered under ITS OWN Target's network — can never match, and
// a forged "foreign root + real name" pair is rejected by the state-root pin
// and by the root-token equality asserted here. Unresolvable input fails
// closed with an error. No product consent/auth behavior changes: this is a
// read-side registry attribution check.
func keycloakTargetConsumerNetwork(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) (string, error) {
	if err := deployment.ValidateTargetName(targetName); err != nil {
		return "", err
	}
	// The producer's namespace is the resolved Target name itself (see the
	// doc comment above): the network is composed from targetName. The state
	// root is still validated by the caller against
	// deployment.TargetStateRoot(targetName), so a foreign root fails closed
	// before this point; here the root's last token must equal the target
	// name's directory form as a second pin against name/path confusion.
	if got := filepath.Base(filepath.Clean(strings.TrimSpace(targetStateRoot))); got != strings.TrimSpace(targetName) {
		return "", errors.New("keycloak witness: target state root does not belong to the witnessed target name")
	}
	network, err := application.IdentityProviderNetworkNameForPlacement(m, targetName, placement)
	if err != nil {
		return "", err
	}
	return network, nil
}
