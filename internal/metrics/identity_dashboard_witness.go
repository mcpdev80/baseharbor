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

	// Registry access and the placement axes (w267 CHANGES 7196719214 item 1,
	// generalised): a SHARED placement's identity record is target-wide — the
	// producer writes OwnerApplication "core" and SharingBoundary "core"
	// regardless of which consumer app is witnessed, and the central List()
	// environment filter would drop that record for every non-"prod" caller
	// (the installation pins environment "prod", keycloak_files.go) — so the
	// shared witness reads the registry directly and asserts the placement
	// axes (Scope, SharingBoundary) explicitly below. An APPLICATION
	// placement's record is owned by the application itself: the witness
	// keeps the PRODUCTION list seam (observability.List with the owning
	// application and the caller's environment, the same route the
	// Prometheus composition and verification use), so the environment
	// isolation card 0428-121 established stays exactly in force here.
	var sources []observability.SignalSource
	if placement.Scope == capability.ScopeShared {
		all, allErr := observability.ListAll()
		if allErr != nil {
			return false, false
		}
		for _, source := range all {
			if source.Kind == observability.SignalMetrics {
				sources = append(sources, source)
			}
		}
	} else {
		policy, policyErr := application.MetricsPolicy(m)
		if policyErr != nil {
			return false, false
		}
		listed, listErr := observability.List(
			observability.SignalMetrics,
			placement,
			[]string{m.Name},
			false,
			policy.Collect[application.MetricsSourceApplicationProvider],
			policy.Collect[application.MetricsSourcePlatformProvider],
			m.Environment,
		)
		if listErr != nil {
			return false, false
		}
		sources = listed
	}
	for _, source := range sources {
		if source.Kind != observability.SignalMetrics {
			continue
		}
		// Registration-identity axis: the witness expects exactly the id the
		// production producer writes for this placement and Target (see
		// keycloakMetricsRegistrationIDAt; w267 CHANGES 7196719214 item 1).
		// The per-Target binding itself is asserted on the injective state-
		// root attribution below, never on this field alone.
		if source.Provider != capability.ProviderKeycloak || source.ID != expectedID {
			continue
		}
		// The producer writes OwnerApplication verbatim (identityprovider
		// registerKeycloakSignals: r.app.Name). The shared placement's
		// installation pins app.Name to "core" only on the FILES path
		// (keycloak_files.go overrides a local copy for the network
		// grammar); the REGISTRATION carries the caller's owning
		// application (r.app.Name), so the witness pins exactly that —
		// the same identity RegisterProviderSignals persists (w267
		// CHANGES 7196719214 item 1: the guard must hold for actual
		// production records; the seed mirrors this registration).
		if source.Scope != placement.Scope || source.OwnerApplication != m.Name {
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
		// Target attribution check (w267 CHANGES 7194027071, collision class
		// of 7196719214): besides the Network equality above, the record
		// must carry the production attribution token — in the observability
		// registry's canonical runtime://<project>/<service> grammar (the
		// same grammar the environment resolver keys on, internal/
		// observability/environment.go), the service half naming
		// "keycloak@<state-root directory>:9000" exactly as the CURRENT
		// producer registers it (internal/identityprovider
		// keycloakSignalTargetAttribution). The state-root DIRECTORY is
		// unique per validated Target name (deployment.TargetStateRoot keys
		// the directory on the exact name), so the attribution is injective
		// even for the names "a.b" and "a-b" whose projectToken and
		// consumer-network forms normalize identically (dots to hyphens,
		// identity_network.go:40, project_names.go:41-48): a foreign
		// Target's signal can never satisfy this witness, while the
		// matching Target's own attributed record stays eligible.
		// Unattributed (plain endpoint, incl. stamped-legacy) or foreign-
		// attributed records fail closed.
		if recordRoot := keycloakRecordTargetRoot(source, expectedRoot, targetName); recordRoot == "" || filepath.Clean(recordRoot) != filepath.Clean(expectedRoot) {
			continue
		}
		metricPresent = true
		metricCurrent = true
	}
	return metricPresent, metricCurrent
}

// keycloakMetricsRegistrationIDAt returns the registry identity the witness
// expects the native keycloak-metrics signal under. Per the 0428-1707
// scoping the registration id is "<provider>:<project>" (disjoint from the
// legacy literal "keycloak-metrics"): the producer's id is "keycloak:" +
// files.Project — the Target's shared identity installation project
// (identityprovider keycloakStateIdentity → runtime
// TargetInstallationProject(namespace), i.e. SharedProjectName("<target>-
// core")) for the shared placement, and the application canonical project
// for the application placement. The witness derives the same grammar from
// the validated Target name; a name or root that does not resolve yields ""
// and keycloakMetricSignals fails closed (see the expectedID == "" guard
// above). The id itself is project-scoped and NOT the per-Target binding:
// Target attribution is asserted in keycloakMetricSignals on the record's
// Network (target-scoped lookup scope) AND the injective state-root
// attribution token in the record's canonical runtime Target (w267 HOLD
// 7192567805, CHANGES 7194027071 / 7196719214).
func keycloakMetricsRegistrationIDAt(m application.Manifest, placement capability.ProviderPlacement, targetStateRoot, targetName string) string {
	if err := deployment.ValidateTargetName(targetName); err != nil {
		return ""
	}
	switch placement.Scope {
	case capability.ScopeShared:
		// The producer registers under the Target's installation project
		// (identityprovider keycloakStateIdentity's shared branch).
		return string(capability.ProviderKeycloak) + ":" + bhruntime.TargetInstallationProject(targetName)
	case capability.ScopeApplication:
		// The app-scoped Keycloak installation registers under the OWNING
		// application's canonical project whose Target token is the
		// producer's resolved Target namespace (identityprovider
		// keycloakStateIdentity's application branch: runtime
		// ApplicationProjectName(namespace, <app>-identity, env)). The
		// witness derives the identical grammar from the Target IT is asked
		// about — targetName IS the producer's namespace on the production
		// status face (cmd/baha/app_status_result.go passes
		// resolved.Target.Name), which is where this placement's producer
		// runs.
		return string(capability.ProviderKeycloak) + ":" + bhruntime.ApplicationProjectName(targetName, m.Name+"-identity", m.Environment)
	default:
		return ""
	}
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
// with the service half carrying the production attribution token this
// runtime's Keycloak producer writes (internal/identityprovider
// keycloakSignalAttributedService): "keycloak@<state-root name>:9000",
// where the token is the Target's state-root DIRECTORY NAME — the exact
// validated Target name deployment.TargetStateRoot keyed the directory on.
// That name is the INJECTIVE component: the ID project and the consumer
// network run their names through projectToken normalization (dots mapped
// to hyphens), so the valid names "a.b" and "a-b" collide on those fields
// yet attribute to distinct roots here (w267 CHANGES 7194027071,
// 7196719214). The token must equal the witnessed root's directory name,
// and the project half must name an installation consistent with that
// root. Pre-correction plain-endpoint records ("keycloak:9000", no
// attribution token), foreign-attributed records, and cross-grammar
// fabrications (root and project that no production installation can
// write together) resolve NO root and fail closed.
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
	// The attribution grammar of the CURRENT production producer
	// (keycloakSignalAttributedService): keycloak@<state-root name>:9000,
	// where the name is the validated Target name. The witness's network
	// lookup scope embeds the same raw Target name as its last segment
	// (identity_network.go appends the namespace verbatim), so real
	// production records agree on both axes by construction; a fabricated
	// mismatch fails closed in the network guard below (w267 HOLD
	// 7192567805).
	const marker = "keycloak@"
	if !strings.HasPrefix(service, marker) || !strings.HasSuffix(service, ":9000") {
		return ""
	}
	token := strings.TrimSuffix(strings.TrimPrefix(service, marker), ":9000")
	if err := deployment.ValidateTargetName(token); err != nil {
		return ""
	}
	if token != witnessName || token != filepath.Base(witnessed) {
		return ""
	}
	// Structural project check: the producer writes its OWN registration
	// project (identityprovider sharedKeycloakProject / keycloakStateIdentity
	// grammar). The record's project must be the installation project of the
	// SAME Target; a root/token mix no production installation can write
	// fails closed. The injective binding itself is the equality above.
	if !keycloakProjectNamesTarget(project, token) {
		return ""
	}
	return witnessed
}

// keycloakProjectNamesTarget reports whether a registry project is the
// production identity installation project of the Target whose state-root
// directory name is targetName. It accepts exactly the two project
// grammars identityprovider keycloakStateIdentity writes: the shared
// installation project runtime.TargetInstallationProject(name)
// ("bh-<projectToken(name-core)>-shared") and the application-scope project
// runtime.ApplicationProjectName(name, <app>-identity, <env>) — for the
// witness's Target-scoped check only the leading Target token of the
// application grammar is identity-relevant, so the app/env segments are
// structurally free. projectToken normalizes dots to hyphens, so this
// grammar is by construction NOT the collision separator
// (SharedProjectName("a.b-core") == SharedProjectName("a-b-core") — review
// 7196719214 item 2): the injective binding lives in the attribution token
// (keycloakRecordTargetRoot). An exact grammar match is required —
// anything else fails closed.
func keycloakProjectNamesTarget(project, targetName string) bool {
	project = strings.TrimSpace(project)
	if err := deployment.ValidateTargetName(targetName); err != nil {
		return false
	}
	if project == "" {
		return false
	}
	if project == bhruntime.TargetInstallationProject(targetName) {
		return true
	}
	// Application-scope installation project "bh-<token(target)>-<app>-<env>":
	// the Target token is the first segment after the "bh-" prefix.
	const prefix = "bh-"
	if !strings.HasPrefix(project, prefix) {
		return false
	}
	token := strings.TrimPrefix(project, prefix)
	if idx := strings.IndexByte(token, '-'); idx >= 0 {
		token = token[:idx]
	}
	return token != "" && token == projectTokenFor(targetName)
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

// keycloakRecordEndpoint accepts the production forms of the Keycloak
// signal's runtime endpoint in the record's Target field: the plain service
// endpoint "keycloak:9000" (the pre-attribution form older deployed records
// carry), the canonical attribution grammar the CURRENT producer writes
// (runtime://<project>/keycloak@<root>:9000), and the stamped legacy
// runtime://<project>/keycloak form the registry migration produces
// (internal/observability migration.go stampLegacyTarget strips the port).
// All three express the same endpoint identity; the per-Target attribution
// check (keycloakRecordTargetRoot) decides eligibility — the plain form
// resolves NO root and fails closed.
func keycloakRecordEndpoint(target string) bool {
	value := strings.TrimSpace(target)
	if value == "keycloak:9000" {
		return true
	}
	_, service, ok := observability.ParseRuntimeTarget(value)
	if !ok {
		return false
	}
	service = strings.TrimSpace(service)
	if service == "keycloak" || service == "keycloak:9000" {
		return true
	}
	return strings.HasPrefix(service, "keycloak@") && strings.HasSuffix(service, ":9000")
}
