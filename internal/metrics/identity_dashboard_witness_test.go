package metrics

import (
	"os"
	"path/filepath"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// seedKeycloakMetricsSourceForTarget registers, in an isolated observability
// registry, the exact record shape the production Keycloak runtime writes:
// the registration goes through the production seam
// observability.RegisterProviderSignals with the same struct the realization
// composes (internal/identityprovider keycloak_realization.go): ID
// "keycloak:<installation project>", Network = the producer's consumer
// network, Target = observability.RuntimeTarget(<installation project>,
// "keycloak@<state root>:9000"), and the canonical descriptor fields
// (capability/reference.go). The project-scoped ID grammar stays production
// (0428-1707 scoping); the per-Target attribution lives in the record's
// Target field. ScopeApplication keeps the match on OwnerApplication (the
// witness asserts per-Target binding on the state root and the endpoint, not
// on a shared-boundary), so the seed matches the same way the shared
// production placement does.
func seedKeycloakMetricsSourceForTarget(t *testing.T, owner string, project string, stateRoot string, targetName string) observability.SignalSource {
	t.Helper()
	network := witnessTargetNetwork(t, owner, stateRoot, targetName)
	attribution := witnessAttributedService(stateRoot)
	registration := observability.ProviderSignalRegistration{
		ID:              "keycloak:" + project,
		Descriptor:      capability.KeycloakIntegration,
		Class:           observability.SourceApplicationProvider,
		Scope:           capability.ScopeShared,
		SharingBoundary: "core",
		// The production registration carries the caller's owning
		// application verbatim (registerKeycloakSignals: OwnerApplication
		// r.app.Name); the witness pins the same identity.
		OwnerApplication: owner,
		Enabled:          map[observability.SignalKind]bool{observability.SignalMetrics: true},
		Signals: map[string]observability.ProviderSignalRuntime{
			"keycloak-metrics": {
				Network: network,
				Target:  observability.RuntimeTarget(project, attribution),
			},
		},
	}
	if err := observability.RegisterProviderSignals(registration); err != nil {
		t.Fatalf("seed keycloak metrics source through the production registration seam: %v", err)
	}
	sources, err := observability.ListAll()
	if err != nil {
		t.Fatalf("read back the registered keycloak metrics source: %v", err)
	}
	for _, source := range sources {
		if source.ID == "keycloak:"+project && source.Provider == capability.ProviderKeycloak && source.Kind == observability.SignalMetrics {
			return source
		}
	}
	t.Fatalf("registered keycloak metrics source is not in the registry")
	return observability.SignalSource{}
}

// witnessAttributedService is the test-side twin of the production
// attribution token (internal/identityprovider
// keycloakSignalAttributedService): keycloak@<state-root name>:9000, the
// state root's directory name = the exact validated Target name.
func witnessAttributedService(stateRoot string) string {
	return "keycloak@" + filepath.Base(filepath.Clean(stateRoot)) + ":9000"
}

// witnessTargetNetwork is the registration NETWORK the seed writes into a
// record, mirroring the production producer (KeycloakFiles.ConsumerNetwork):
// the identity consumer network of the Target's Core resource project
// ("baseharbor-identity-core-baseharbor-<token>-core"), the same grammar
// keycloakTargetConsumerNetwork looks the record up under. The token form
// maps dots to hyphens (identity_network.go:40), so the seeds for the valid
// names "a.b" and "a-b" carry ONE network — the collision w267 CHANGES
// (comment 7194027071) exercises; the per-Target separation comes from the
// injective state-root attribution token in the record's runtime:// Target,
// not from this field.
func witnessTargetNetwork(t *testing.T, owner, stateRoot, targetName string) string {
	t.Helper()
	if got := filepath.Base(filepath.Clean(stateRoot)); got != targetName {
		t.Fatalf("seed state root %q does not belong to target %q", stateRoot, targetName)
	}
	network, err := application.IdentityProviderNetworkNameForPlacement(
		application.New(owner, "dev", false, false, false),
		bhruntime.SharedResourceProjectName(targetName+"-core"),
		capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	)
	if err != nil {
		t.Fatalf("identity consumer network: %v", err)
	}
	return network
}

// manifestForIdentityDashboardWitness is the minimal manifest the witness needs:
// the Keycloak identity provider selected, a Keycloak metrics source, and the
// observability-management-UI + Perses surface the status face gates on.
func manifestForIdentityDashboardWitness(name, project string) application.Manifest {
	m := application.New(name, "dev", false, false, false)
	m = application.WithIdentity(m)
	m = application.WithMetricsSource(m, "api", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// TestIdentityDashboardWitnessPositiveMatchesConcreteTarget is the real
// queried fixture for 0428-078: a Keycloak native metrics signal registered for
// the exact Target the witness is asked about must make the witness
// non-forgeably available. Per-Target binding is asserted on the production
// state root (TargetStateRoot(targetName)) and the production Keycloak endpoint
// ("keycloak:9000"), so this is not a bare true assertion — remove either and
// the witness must fail closed.
func TestIdentityDashboardWitnessPositiveMatchesConcreteTarget(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	target := deployment.ResolvedTarget{Name: "docker-dev", RuntimeProvider: "docker"}
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil || stateRoot == "" {
		t.Fatalf("resolve target state root: %v", err)
	}

	// The producer registers under the Target's installation project
	// (identityprovider keycloakStateIdentity → runtime
	// TargetInstallationProject(namespace)).
	owner := "identity-provider"
	ownerProject := bhruntime.TargetInstallationProject(target.Name)
	seedKeycloakMetricsSourceForTarget(t, owner, ownerProject, stateRoot, target.Name)
	m := manifestForIdentityDashboardWitness(owner, owner)

	placement := capability.ProviderPlacement{
		Scope:     capability.ScopeShared,
		Ownership: capability.OwnershipBaseHarbor,
	}
	witness := IdentityDashboardWitnessAt(m, placement, stateRoot, target.Name)
	if !witness.NonForgeableAvailable() {
		t.Fatalf("the witness must be non-forgeably available for the concrete matching Target, got %+v", witness)
	}
}

// TestIdentityDashboardWitnessTwoValidTargetsIsolation is the two-valid-Target
// regression the w267 HOLD (comment 7192567805, requirement 2) demands: TWO
// real, resolvable Target roots in one shared registry, each carrying the
// production registration shape (same installation-project ID grammar, same
// endpoint service, same descriptor fields) but attributed to its own Target
// through the injective state-root component of the canonical runtime
// Target (internal/identityprovider keycloakSignalAttributedService), on
// top of the consumer network the producer registers. The FOREIGN Target's
// registered signal must NOT
// make this Target eligible, while the MATCHING Target's signal must keep it
// eligible even with the foreign record present. Unlike the state-root
// mismatch probe, this exercises two VALID roots: only a genuine per-Target
// attribution binding passes.
func TestIdentityDashboardWitnessTwoValidTargetsIsolation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	owner := "identity-provider"
	realRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || realRoot == "" {
		t.Fatalf("resolve this target state root: %v", err)
	}
	foreignRoot, err := deployment.TargetStateRoot("other-dev")
	if err != nil || foreignRoot == "" {
		t.Fatalf("resolve foreign target state root: %v", err)
	}
	if filepath.Clean(foreignRoot) == filepath.Clean(realRoot) {
		t.Fatal("the two valid Target roots must be distinct")
	}
	m := manifestForIdentityDashboardWitness(owner, owner)
	placement := capability.ProviderPlacement{
		Scope:     capability.ScopeShared,
		Ownership: capability.OwnershipBaseHarbor,
	}

	// Phase 1 — ONLY the foreign Target's signal exists in the registry.
	// Same installation-project ID grammar, same endpoint, a valid foreign
	// root: the foreign Target's registered signal must NOT make THIS
	// Target eligible.
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("other-dev"), foreignRoot, "other-dev")
	if w := IdentityDashboardWitnessAt(m, placement, realRoot, "docker-dev"); w.NonForgeableAvailable() {
		t.Fatalf("the foreign Target's registered signal must not make this Target eligible, got %+v", w)
	}

	// Phase 2 — THIS Target's own signal is registered as well. The matching
	// Target must be eligible; the witness must not flip back to ineligible
	// because a foreign record also exists.
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("docker-dev"), realRoot, "docker-dev")
	if w := IdentityDashboardWitnessAt(m, placement, realRoot, "docker-dev"); !w.NonForgeableAvailable() {
		t.Fatalf("the matching Target's own signal must keep it eligible alongside the foreign record, got %+v", w)
	}

	// A witness whose state root resolves to a DIFFERENT target name than the
	// one it is asked about stays fail-closed (pin + attribution both bound).
	if w := IdentityDashboardWitnessAt(m, placement, foreignRoot, "docker-dev"); w.NonForgeableAvailable() {
		t.Fatalf("a state root that resolves to a different target name must fail closed, got %+v", w)
	}
}

// TestIdentityDashboardWitnessTwoValidDottedTargetsIsolation is the w267
// CHANGES (comment 7194027071, requirement 3) collision case: TWO valid
// Targets whose names normalize to the SAME network token — "a.b" and "a-b"
// (deployment.ValidateTargetName admits dots;
// application.IdentityProviderNetworkNameForPlacement maps dots to hyphens,
// identity_network.go:40) — share one plain consumer-network grammar output
// AND one normalized installation project (runtime
// TargetInstallationProject maps dots to hyphens the same way, the premise
// w267 CHANGES 7196719214 item 2 names). The witness's attribution check
// carries the RAW Target name in the injective state-root component of the
// record's canonical runtime Target, so the foreign Target's registered
// signal must fail closed here while the matching Target's own signal stays
// eligible. This is the residual forgery class the ordinary-name regression
// cannot express.
func TestIdentityDashboardWitnessTwoValidDottedTargetsIsolation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	owner := "identity-provider"
	dottedRoot, err := deployment.TargetStateRoot("a.b")
	if err != nil || dottedRoot == "" {
		t.Fatalf("resolve a.b state root: %v", err)
	}
	hyphenRoot, err := deployment.TargetStateRoot("a-b")
	if err != nil || hyphenRoot == "" {
		t.Fatalf("resolve a-b state root: %v", err)
	}
	if filepath.Clean(dottedRoot) == filepath.Clean(hyphenRoot) {
		t.Fatal("the two valid Target roots must be distinct")
	}
	// The normalization collision premise: after the network grammar's dot
	// map the plain Target names share one network token — the property the
	// raw-name suffix must break.
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	plainDotted, err := application.IdentityProviderNetworkNameForPlacement(
		application.New(owner, "dev", false, false, false), "a.b", placement)
	if err != nil {
		t.Fatalf("plain a.b network: %v", err)
	}
	plainHyphen, err := application.IdentityProviderNetworkNameForPlacement(
		application.New(owner, "dev", false, false, false), "a-b", placement)
	if err != nil {
		t.Fatalf("plain a-b network: %v", err)
	}
	if plainDotted != plainHyphen {
		t.Fatalf("test premise wrong: the plain grammar already separates a.b from a-b (%q vs %q)", plainDotted, plainHyphen)
	}

	m := manifestForIdentityDashboardWitness(owner, owner)

	// The FOREIGN Target ("a-b") registers its signal: same installation-
	// project ID grammar, same endpoint, colliding plain network. It must
	// NOT make the dotted Target eligible.
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("a-b"), hyphenRoot, "a-b")
	if w := IdentityDashboardWitnessAt(m, placement, dottedRoot, "a.b"); w.NonForgeableAvailable() {
		t.Fatalf("the normalization-colliding foreign Target's signal must not make a.b eligible, got %+v", w)
	}
	// The MATCHING Target's own signal keeps it eligible, foreign record
	// present.
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("a.b"), dottedRoot, "a.b")
	if w := IdentityDashboardWitnessAt(m, placement, dottedRoot, "a.b"); !w.NonForgeableAvailable() {
		t.Fatalf("a.b's own signal must keep it eligible alongside the colliding foreign record, got %+v", w)
	}
}

// TestIdentityDashboardWitnessForeignTargetFailsClosed asserts the per-Target
// binding the review requires: the same signal registered for a DIFFERENT
// Target's state root must NOT satisfy a witness asked about this Target. The
// production ID is project-scoped, so a shared-boundary neighbour cannot present
// a foreign Target's signal as this Target's — the witness fails closed on the
// state-root mismatch.
func TestIdentityDashboardWitnessForeignTargetFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	owner := "identity-provider"
	// Register for the real target's state root.
	realRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || realRoot == "" {
		t.Fatalf("resolve real target state root: %v", err)
	}
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("docker-dev"), realRoot, "docker-dev")
	m := manifestForIdentityDashboardWitness(owner, owner)
	placement := capability.ProviderPlacement{
		Scope:     capability.ScopeShared,
		Ownership: capability.OwnershipBaseHarbor,
	}

	// A foreign Target's state root (unresolvable here, and it never belongs
	// to "docker-dev") must not satisfy the witness asked about the real
	// target — neither the root pin nor the per-Target attribution can hold
	// for it.
	witness := IdentityDashboardWitnessAt(m, placement, filepath.Join(realRoot, "-foreign"), "docker-dev")
	if witness.NonForgeableAvailable() {
		t.Fatalf("a foreign target state root must not make the witness available, got %+v", witness)
	}

	// And the real witness must still be available (the foreign case did not
	// disturb the seeded real binding).
	witnessReal := IdentityDashboardWitnessAt(m, placement, realRoot, "docker-dev")
	if !witnessReal.NonForgeableAvailable() {
		t.Fatalf("the real target binding must remain available after the foreign probe, got %+v", witnessReal)
	}
}

// TestIdentityDashboardWitnessRequiresTargetStateRoot is the fail-closed guard
// when the state root cannot be resolved to a concrete Target: the witness
// must never claim the real signal is bound to an unknown Target.
func TestIdentityDashboardWitnessRequiresTargetStateRoot(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	owner := "identity-provider"
	realRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || realRoot == "" {
		t.Fatalf("resolve real target state root: %v", err)
	}
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("docker-dev"), realRoot, "docker-dev")
	m := manifestForIdentityDashboardWitness(owner, owner)
	placement := capability.ProviderPlacement{
		Scope:     capability.ScopeShared,
		Ownership: capability.OwnershipBaseHarbor,
	}

	// Empty state root (unresolvable) must fail closed.
	if w := IdentityDashboardWitnessAt(m, placement, "", "docker-dev"); w.NonForgeableAvailable() {
		t.Fatalf("an empty target state root must fail closed, got %+v", w)
	}
	// Unresolvable target name must fail closed.
	if w := IdentityDashboardWitnessAt(m, placement, realRoot, ""); w.NonForgeableAvailable() {
		t.Fatalf("an empty target name must fail closed, got %+v", w)
	}
}

// TestIdentityDashboardWitnessExternalPlacementFailsClosed asserts the
// Target-authorization axis: an external OIDC placement (never the
// BaseHarbor-owned Keycloak provider) must fail closed, so a caller cannot make
// availability true by supplying a foreign placement.
func TestIdentityDashboardWitnessExternalPlacementFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	owner := "identity-provider"
	realRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || realRoot == "" {
		t.Fatalf("resolve real target state root: %v", err)
	}
	seedKeycloakMetricsSourceForTarget(t, owner, bhruntime.TargetInstallationProject("docker-dev"), realRoot, "docker-dev")
	m := manifestForIdentityDashboardWitness(owner, owner)
	placement := capability.ProviderPlacement{
		Scope:             capability.ScopeExternal,
		Ownership:         capability.OwnershipExternal,
		ExternalReference: "company-oidc",
	}
	witness := IdentityDashboardWitnessAt(m, placement, realRoot, "docker-dev")
	if witness.NonForgeableAvailable() {
		t.Fatalf("an external placement must fail closed, got %+v", witness)
	}
}

// TestIdentityDashboardWitnessNonEmptyRegistrationIDIsTargetScoped pins the
// production-correct registration identity (Finding 2): the witness's
// expectedID is "<provider>:<project>" and, combined with the per-Target state
// root and endpoint guards, is genuinely target-scoped. A foreign Target cannot
// present a signal it does not own.
func TestIdentityDashboardWitnessNonEmptyRegistrationIDIsTargetScoped(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	m := manifestForIdentityDashboardWitness("identity-provider", "identity-provider")
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	root, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || root == "" {
		t.Fatalf("resolve target state root: %v", err)
	}
	// The production registration id is "keycloak:" + the producer's OWN
	// installation project (identityprovider keycloakStateIdentity →
	// runtime TargetInstallationProject(namespace), w267 CHANGES
	// 7196719214 item 1: the witness must expect the exact grammar the
	// producer-to-registry path writes, not the consumer manifest's name).
	wantID := "keycloak:" + bhruntime.TargetInstallationProject("docker-dev")
	if id := keycloakMetricsRegistrationIDAt(m, placement, root, "docker-dev"); id != wantID {
		t.Fatalf("registration id = %q, want the production installation-project id %q", id, wantID)
	}
	if empty := keycloakMetricsRegistrationIDAt(m, placement, root, ""); empty != "" {
		t.Fatalf("an empty target name must yield an empty registration id, got %q", empty)
	}
}

// TestIdentityDashboardWitnessRootAttributionResolvers pins the collision
// class w267 CHANGES (comment 7194027071) requires the attribution to
// separate: "a.b" and "a-b" share one consumer-network token (the grammar
// maps dots to hyphens, identity_network.go:40) but their installation
// projects and roots are directory-unique, so each record attributes to its
// OWN root only.
func TestIdentityDashboardWitnessRootAttributionResolvers(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	dottedRoot, err := deployment.TargetStateRoot("a.b")
	if err != nil || dottedRoot == "" {
		t.Fatalf("resolve a.b root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dottedRoot, "providers", "keycloak"), 0o700); err != nil {
		t.Fatalf("materialize a.b identity state: %v", err)
	}
	hyphenRoot, err := deployment.TargetStateRoot("a-b")
	if err != nil || hyphenRoot == "" {
		t.Fatalf("resolve a-b root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(hyphenRoot, "providers", "keycloak"), 0o700); err != nil {
		t.Fatalf("materialize a-b identity state: %v", err)
	}
	for _, tc := range []struct {
		name      string
		project   string
		root      string
		witnessed string
		want      string
	}{
		{"dotted attributes to dotted", bhruntime.TargetInstallationProject("a.b"), dottedRoot, dottedRoot, dottedRoot},
		{"hyphen attributes to hyphen", bhruntime.TargetInstallationProject("a-b"), hyphenRoot, hyphenRoot, hyphenRoot},
		{"dotted record never attributes to hyphen", bhruntime.TargetInstallationProject("a.b"), dottedRoot, hyphenRoot, ""},
		{"hyphen record never attributes to dotted", bhruntime.TargetInstallationProject("a-b"), hyphenRoot, dottedRoot, ""},
		// Review 7196719214 item 2 premise proof: the normalized project
		// grammar is collision-LOSSY — SharedProjectName("a.b-core") ==
		// SharedProjectName("a-b-core") — so the a-b installation project
		// with an a.b ROOT is rejected: the witness requires the record's
		// project to be the installation project of the root it attributes
		// to, and only a target-consistent production record stays eligible.
		{"colliding project never names a.b even with the dotted root", bhruntime.TargetInstallationProject("a-b"), dottedRoot, dottedRoot, ""},
	} {
		// The record shape the CURRENT producer writes: installation project
		// in the grammar position, injective state-root token in the service
		// position (internal/identityprovider keycloak_realization.go
		// keycloakSignalTargetAttribution).
		source := observability.SignalSource{Target: observability.RuntimeTarget(tc.project, witnessAttributedService(tc.root))}
		if got := keycloakRecordTargetRoot(source, tc.witnessed, filepath.Base(tc.witnessed)); got != tc.want {
			t.Fatalf("%s: attribution = %q, want %q", tc.name, got, tc.want)
		}
	}
	// The unattributed plain-endpoint form resolves no root at all.
	if got := keycloakRecordTargetRoot(observability.SignalSource{Target: "keycloak:9000"}, dottedRoot, "a.b"); got != "" {
		t.Fatalf("unattributed record must resolve no root, got %q", got)
	}
}
