package metrics

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	// w267 HOLD correction (comment 7192567805), collision class per the
	// CHANGES round (comment 7194027071): the registration ID is project-
	// scoped and identical for every Target; the endpoint is shared; and the
	// consumer network is normalization-lossy ("a.b" and "a-b" map dots to
	// hyphens identically, identity_network.go:40), so NONE of the three
	// fields attributes a record to the concrete Target being witnessed. The
	// witness binds the decision to the ACTUAL resolved Target through the
	// root-derived compose project the observability registry attributes
	// sources with (the runtime://<project>/<service> grammar its
	// environment resolver keys on, internal/observability/environment.go):
	// keycloakRecordTargetRoot restores the record's Target root — its state
	// directory must exist under this installation (deployment.TargetStateRoot
	// + providers/keycloak) — and the guard below requires it to equal the
	// root THIS witness resolved from targetName. Roots are directory-unique
	// per validated Target name, so the comparison is injective for dotted
	// and hyphenated names alike; a foreign Target's signal can never make
	// this Target eligible, the matching Target's attributed signal keeps it
	// eligible, and unattributed records fail closed. The Network equality
	// check stays in force as the real-network guard w267 requires. No product
	// consent/auth behavior changes: this is a read-side registry attribution
	// check.
	expectedNetwork, expectedRoot, networkErr := keycloakTargetConsumerNetwork(m, placement, targetStateRoot, targetName)
	if networkErr != nil || expectedNetwork == "" || expectedRoot == "" {
		return false, false
	}
	expectedID := keycloakMetricsRegistrationIDAt(m, placement, targetStateRoot, targetName)
	if expectedID == "" {
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
		if !keycloakRecordEndpoint(source.Target) ||
			source.Mode != expectedSignal.Mode ||
			source.Protocol != expectedSignal.Protocol ||
			source.Path != expectedSignal.Path ||
			source.Verification != expectedSignal.Verification {
			continue
		}
		// Real-network check (kept from the HOLD round): the record's Network
		// must equal this Target's installation consumer network.
		if strings.TrimSpace(source.Network) != expectedNetwork || expectedNetwork == "" {
			continue
		}
		// Target attribution check (w267 CHANGES 7194027071 collision
		// class): besides the Network equality above, the record must be
		// attributed — in the observability registry's canonical
		// runtime://<project>/<service> grammar (the same grammar the
		// environment resolver keys on, internal/observability/
		// environment.go; keycloakRecordTargetRoot) — to exactly THIS
		// witness's installation project, composed from the Target's state
		// ROOT DIRECTORY name. The root directory is unique per validated
		// Target name (deployment.TargetStateRoot), so the attribution is
		// injective even for the names "a.b" and "a-b" whose projectToken
		// and consumer-network forms normalize identically (dots to
		// hyphens, identity_network.go:40): a foreign Target's signal can
		// never satisfy this witness, while the matching Target's own
		// attributed signal keeps it eligible. Unattributed or
		// foreign-attributed records fail closed.
		if recordRoot := keycloakRecordTargetRoot(source, expectedRoot, targetName); recordRoot == "" || filepath.Clean(recordRoot) != filepath.Clean(expectedRoot) {
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
// keycloakMetricsRegistrationIDAt returns the registry identity the witness
// expects the native keycloak-metrics signal under. Per the 0428-1707
// scoping the registration id is "<provider>:<project>" (disjoint from the
// legacy literal "keycloak-metrics"): the producer's id is "keycloak:" +
// files.Project. The project the witness expects is the CONSUMER-side
// canonical project for this Target and manifest — for the shared placement
// the producer's Core installation path passes app.Name "core" (identity-
// provider/core.go), but the app-scoped consumer registers under the app's
// OWN canonical project (keycloakStateIdentity's application branch:
// runtime.ApplicationProjectName(namespace, <app>-identity, env)); the
// manifest-name form survives as the conservative local-face fallback. The
// id itself is project-scoped and NOT the per-Target binding: Target
// attribution is asserted in keycloakMetricSignals on BOTH the record's
// Network (target-scoped lookup scope) AND the record's root attribution
// (w267 HOLD 7192567805, CHANGES 7194027071: the collision class "a.b" vs
// "a-b" separates on the root halves, which are directory-unique). A missing
// or unresolvable binding returns empty, which makes keycloakMetricSignals
// fail closed (see the expectedID == "" guard above).
func keycloakMetricsRegistrationIDAt(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) string {
	if strings.TrimSpace(targetName) == "" {
		return ""
	}
	// The registration id grammar stays the production project form
	// "<provider>:<project>" (0428-1707 scoping, disjoint from the legacy
	// literal "keycloak-metrics"): for the shared placement the producer's
	// project is the Target's installation project (identityprovider
	// keycloakStateIdentity → runtime.SharedProjectName(namespace+"-core")),
	// and the manifest name is the conservative face-compatible fallback.
	// The id itself is project-scoped and NOT the per-Target binding; the
	// Target attribution is asserted in keycloakMetricSignals on the record's
	// Network (target-scoped lookup scope) AND the record's root attribution
	// (w267 HOLD 7192567805, CHANGES 7194027071).
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
func keycloakTargetConsumerNetwork(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) (networkScope, targetRoot string, err error) {
	if err := deployment.ValidateTargetName(targetName); err != nil {
		return "", "", err
	}
	// The state root's last token must equal the target name's directory
	// form: the caller already pinned the root to
	// deployment.TargetStateRoot(targetName); this is a second pin against
	// name/path confusion.
	cleanedRoot := filepath.Clean(strings.TrimSpace(targetStateRoot))
	if got := filepath.Base(cleanedRoot); got != strings.TrimSpace(targetName) {
		return "", "", errors.New("keycloak witness: target state root does not belong to the witnessed target name")
	}
	switch placement.Scope {
	case capability.ScopeShared:
		// The shared placement's lookup scope is the identity consumer
		// network the producer registers with: the installation network
		// "baseharbor-identity-core" extended with the Target's Core
		// resource project "baseharbor-<token>-core"
		// (runtime.SharedResourceProjectName). Distinct Targets get
		// distinct networks EXCEPT for names that normalize to one token
		// ("a.b" vs "a-b", identity_network.go:40 maps dots to hyphens).
		// The state root is therefore returned as a SEPARATE, injective
		// attribution half: the witness requires BOTH the record's Network
		// equals this scope AND the record's canonical-project attribution
		// (its runtime:// Target grammar, the same grammar the
		// observability environment resolver keys on,
		// internal/observability/environment.go:112+) names exactly THIS
		// Target's installation project. Distinct roots always produce
		// distinct installation projects, so the colliding names separate;
		// unattributed or foreign-attributed records fail closed. No
		// product consent/auth behavior changes.
		scopeNetwork, scopeErr := application.IdentityProviderNetworkNameForPlacement(m, bhruntime.SharedResourceProjectName(strings.TrimSpace(targetName)+"-core"), placement)
		if scopeErr != nil {
			return "", "", scopeErr
		}
		return scopeNetwork, cleanedRoot, nil
	default:
		return "", "", errors.New("keycloak witness: managed identity provider requires shared or application placement")
	}
}

// keycloakRecordTargetRoot resolves the concrete Target state root a
// registry record is attributable to. A record is attributable only in the
// observability registry's canonical compose-project grammar,
// runtime://<project>/<service> (ParseRuntimeTarget — the grammar the
// environment resolver keys on, internal/observability/environment.go),
// with BOTH halves pinned to the witnessed Target: the service must equal
// the signal's realization endpoint (keycloakSignalRuntimeService) and the
// project must equal the witnessed root's installation project
// (keycloakInstallationProject, composed from the state root's DIRECTORY
// name, which is unique per Target — deployment.TargetStateRoot keys the
// directory on the exact validated name). The project-scoped ID, the plain
// endpoint literal and the consumer network all carry no root identity and
// are NOT accepted as attribution (w267 HOLD 7192567805 / CHANGES
// 7194027071: the normalization-colliding names "a.b" and "a-b" share one
// network token yet attribute to distinct roots here). An unattributed or
// foreign-attributed record returns "" and the witness fails closed.
func keycloakRecordTargetRoot(source observability.SignalSource, witnessedRoot, witnessName string) string {
	project, service, ok := observability.ParseRuntimeTarget(source.Target)
	if !ok {
		return ""
	}
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	witnessed := filepath.Clean(witnessedRoot)
	if witnessName = strings.TrimSpace(witnessName); witnessName == "" || witnessName != filepath.Base(witnessed) {
		return ""
	}
	// Canonical attribution: the runtime://<project>/<service> grammar (the
	// observability registry's own project-identity form, internal/
	// observability registry.go RuntimeTarget / environment.go), with the
	// service equal to this signal's realization endpoint and the project
	// equal to the TARGET-ATTRIBUTED installation project —
	// keycloakInstallationProject, composed from the witnessed state root's
	// DIRECTORY name, which is unique per Target (deployment.TargetStateRoot
	// keys the directory on the exact validated name). The plain project
	// literal "identity-provider" and the raw-name token form
	// SharedProjectName("a.b-core") == SharedProjectName("a-b-core") carry no
	// root identity and fail closed: attribution is injective because it is
	// rooted at the directory, never at the normalization-lossy token.
	if service != keycloakSignalRuntimeService {
		return ""
	}
	if project == keycloakInstallationProject(witnessed) {
		return witnessed
	}
	return ""
}

const keycloakSignalRuntimeService = "keycloak:9000"

// keycloakInstallationProject is the single attribution grammar of the
// witness: the Target's shared identity installation project — the shape
// runtime.SharedProjectName("<target>-core") produces ("bh-<token>-core-
// shared", internal/runtime project_names.go; the project
// internal/identityprovider keycloakStateIdentity computes for a Target's
// Core installation) — composed with the WITNESSED ROOT'S DIRECTORY name
// kept VERBATIM in the token position. The projectToken normalization maps
// dots to hyphens, so the names "a.b" and "a-b" would share one token;
// their state-root DIRECTORIES differ (deployment.TargetStateRoot keys the
// directory on the exact validated name), so retaining the directory name
// makes the attribution INJECTIVE in the Target: "bh-a.b-core-shared" vs
// "bh-a-b-core-shared". A foreign Target's record can never name this
// witness's project, and the matching Target's attributed record always
// does. No product consent/auth behavior changes.
func keycloakInstallationProject(witnessedRoot string) string {
	name := filepath.Base(filepath.Clean(witnessedRoot))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	return "bh-" + name + "-core-shared"
}

// projectTokenFor mirrors internal/runtime.projectToken (unexported):
// lowercase, dots to hyphens, trimmed — so a Target whose raw name differs
// from its token form is still matched structurally, never by accident.
func projectTokenFor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "local"
	}
	return value
}

// keycloakRecordEndpoint accepts the two production forms of the Keycloak
// signal's runtime endpoint in the record's Target field: the plain service
// endpoint "keycloak:9000" (the form the compose realization writes) and the
// runtime://<project>/keycloak:9000 attribution grammar the observability
// registry exposes for target-attributed registrations. Both express the
// same endpoint identity; attribution decides per Target.
func keycloakRecordEndpoint(target string) bool {
	value := strings.TrimSpace(target)
	if value == "keycloak:9000" {
		return true
	}
	project, service, ok := observability.ParseRuntimeTarget(value)
	return ok && strings.TrimSpace(project) != "" && service == "keycloak:9000"
}
