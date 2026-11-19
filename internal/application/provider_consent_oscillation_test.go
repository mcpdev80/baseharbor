package application

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Regression 0428-119 (hunt 916 befund-1, A-F1 / A.4 / B2 / C3): a registry,
// union, request set and result evidence that never change must not produce a
// different consent identity merely because a transient runtime observation
// flip (docker/podman inspect error folded into the unverifiable default)
// arrived at the plan surface. The digest intentionally binds ABSOLUTE
// authority — registry content, ownership, dependency graph, bindings,
// material inventory and evidence, actual consumer functions, absolute
// validity expiry boundaries. The current-time sample (ObservedAt), the
// unverifiable/verified state classification of unchanged underlying material
// and the derived observation revision are not authority: when they oscillate,
// the post-consent revalidation must recognize the identical plan and apply,
// not livelock on stale-impact-consent forever.
//
// The underlying-state pin (card 0428-116) stays untouched: an actual material
// registry, dependency, binding, ownership, inventory or absolute-expiry
// change still invalidates consent, asserted by the negatives below.

// flipObservation returns a copy of o carrying only the volatile observation
// churn described by kind: "state" flips the unverified-runtime classification
// of the changed provider while its verified material evidence remains, and
// "revision" re-derives the observation revision from a re-sampled clock.
func flipObservation(o ProviderImpactObservation, kind string) ProviderImpactObservation {
	flipped := o
	flipped.Providers = map[string]ProviderImpactFact{}
	for id, f := range o.Providers {
		c := f
		c.Materials = append([]ProviderMaterial(nil), f.Materials...)
		c.Realization.Locations = append([]ResourceLocation(nil), f.Realization.Locations...)
		if kind == "state" && id == "secrets" {
			// Transient recovery: the classifier verified the same running
			// runtime this cycle; the underlying material is unchanged.
			c.Realization.Verified = true
			c.Realization.State = ProviderRunning
			c.Realization.Evidence = "verified runtime inventory"
		}
		if kind == "revision" && id == "secrets" {
			// A re-sampled clock of the SAME verified runtime: the material is
			// unchanged and still verified, only the observation revision
			// churns. The realization still names the running runtime this
			// cycle (State=running), so a Fact=running snapshot stays
			// internally consistent.
			c.Realization.Verified = true
			c.Realization.State = ProviderRunning
			c.Realization.Evidence = "verified runtime inventory"
		}
		flipped.Providers[id] = c
	}
	if kind == "revision" {
		flipped.Revision = "owner/2"
	}
	return flipped
}

// unverifiedFixture models the Layer-1 default (app_core_requirement.go:126-128):
// an unverifiable classification must be internally consistent — the snapshot
// fact says unverifiable and the observation realization is unverified while
// its material evidence remains. Verified realizations must mirror the fact
// exactly (provider_change_validation.go:219).
func unverifiedFixture(t *testing.T) (ResolutionSnapshot, ProviderPlan, ProviderImpactObservation) {
	t.Helper()
	s, u, o := impactFixture(t)
	f := s.Facts["secrets"]
	f.State = ProviderUnverifiable
	s.Facts["secrets"] = f
	// Class (ii): the fact classifies the provider unverifiable AND the
	// observation realization asserts NOTHING concrete (Obs.State stays
	// unverifiable, Verified=false, no Evidence). Nothing is asserted, so the
	// fold leaves the UNKNOWN marker and UNKNOWN is neutral against a pure
	// classification churn (C3, A-F1). This is deliberately a GENUINELY
	// -UNVERIFIABLE realization — NOT class (i): an observation that asserts a
	// concrete state unverified (Verified=false / empty Evidence) is a
	// distinct fail-closed class (see unverifiedUnwitnessedFixture).
	r := o.Providers["secrets"]
	r.Realization.State = ProviderUnverifiable
	r.Realization.Verified = false
	r.Realization.Evidence = ""
	o.Providers["secrets"] = r
	return s, u, o
}

// unverifiedUnwitnessedFixture reproduces the reviewer's exact fail-closed case
// (0428-119 r2, verdict 6997135020): the ORIGINAL Layer-1 projection — a fact
// that classifies the provider UNVERIFIABLE while the observation record still
// carries the concrete running state the adapters saw with Verified=false and
// empty Evidence. The adapters saw material but proved no state. Both folds
// (digest + BuildConsentAuthority) must now fold this to INVALID, so the plan
// is NOT equivalent to a verified one — no asymmetric silent-success class
// between the two folds.
func unverifiedUnwitnessedFixture(t *testing.T) (ResolutionSnapshot, ProviderPlan, ProviderImpactObservation) {
	t.Helper()
	s, u, o := impactFixture(t)
	// The fact keeps the unverifiable Layer-1 classification (original
	// projection, unchanged); the observation realization carries the concrete
	// running state but could not verify it.
	r := o.Providers["secrets"]
	r.Realization.State = ProviderRunning
	r.Realization.Verified = false
	r.Realization.Evidence = ""
	o.Providers["secrets"] = r
	return s, u, o
}

// Verified/Evidence fold (0428-119 r2, verdict 6997135020): an observation that
// could not actually VERIFY its realization (Verified=false, empty Evidence)
// proved no concrete runtime state, so its State must NOT be folded into the
// consent authority as a proven running state. The ORIGINAL projection — a
// Layer-1 unverifiable fact whose observation record still names the concrete
// running state the adapters saw, unverified — is exactly the asymmetric
// silent-success class this fix closes: BOTH the digest fold and the
// BuildConsentAuthority fold now require Verified && Evidence != "" to fold a
// concrete state, otherwise they fold to INVALID. This is the matrix regression: (verified=true,
// evidence!=”) stays equivalent, (verified=false, ”) is NON-equivalent, and
// the two remaining corners (true+empty, false+x) are documented as NON.
func TestConsentDoesNotFoldUnwitnessedRuntimeState(t *testing.T) {
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}

	// verified, evidenced realization: the classifier proved the running runtime.
	sv, uv, ov := impactFixture(t)
	rs := sv
	rsf := rs.Facts["secrets"]
	rsf.State = ProviderRunning
	rs.Facts["secrets"] = rsf
	verifiedPlan, err := DescribeProviderChanges(rs, uv, requests, ov)
	if err != nil {
		t.Fatal(err)
	}
	if verifiedPlan.Authority == nil {
		t.Fatal("plan carries no frozen consent surface")
	}

	// Same underlying material, but the realization could NOT be verified:
	// the fact still says unverifiable (Layer-1 default) while the observation
	// record carries the concrete running state the adapters saw with
	// Verified=false and empty Evidence.
	uu, uuUnion, uuObs := unverifiedUnwitnessedFixture(t)
	unwitnessedPlan, err := DescribeProviderChanges(uu, uuUnion, requests, uuObs)
	if err != nil {
		t.Fatal(err)
	}

	equivalent, err := ProviderImpactPlansConsentEquivalent(verifiedPlan, unwitnessedPlan,
		rs, uu, uuUnion, uuUnion, ov, uuObs)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("an unwitnessed (Verified=false, empty Evidence) runtime state was folded into the consent authority as verified; fail-closed gate lost")
	}
	// The second gate on the revalidation site must agree.
	if err := ValidateProviderImpactRevalidation(unwitnessedPlan, verifiedPlan); err == nil {
		t.Fatal("revalidation second gate accepted an unwitnessed runtime state as equivalent to a verified one")
	}

	// Reverse direction must also fail closed (order symmetry of the fold).
	equivalentRev, err := ProviderImpactPlansConsentEquivalent(unwitnessedPlan, verifiedPlan,
		uu, rs, uuUnion, uuUnion, uuObs, ov)
	if err != nil {
		t.Fatal(err)
	}
	if equivalentRev {
		t.Fatal("an unwitnessed runtime state was accepted as equivalent to a verified one (reverse order)")
	}
}

// TestProviderImpactPlansConsentEquivalentStabilizesClassification is the
// direct unit contract of hunt 916 A-F1/C3: the equivalence surface that the
// post-consent revalidation uses must ignore countdown samples AND the
// unverifiable/verified classification of an unchanged system, while every
// genuine authority change keeps plans unequal (fail closed).
func TestProviderImpactPlansConsentEquivalentStabilizesClassification(t *testing.T) {
	s, u, o := unverifiedFixture(t)
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	runningSnapshot := s
	rsf := runningSnapshot.Facts["secrets"]
	rsf.State = ProviderRunning
	runningSnapshot.Facts["secrets"] = rsf
	flipped, err := DescribeProviderChanges(runningSnapshot, u, requests, flipObservation(o, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if flipped.PlanID == plan.PlanID {
		t.Fatal("regression witness: the raw digest already ignores the classification flip, so the documented window is not modeled here")
	}
	equal, err := ProviderImpactPlansConsentEquivalent(plan, flipped, s, runningSnapshot, u, u, o, flipObservation(o, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Fatal("consent authority changed on a pure classification flip")
	}

	// Negative pins: every real authority change keeps the surfaces unequal.
	// The runningSnapshot above mutated s.Facts in place (shallow copy shares
	// the Facts map), so the revision check rebuilds from a FRESH fixture to
	// isolate "revision churn alone" from the state-flip above.
	rv, ru, ro := unverifiedFixture(t)
	revision := ro
	revision.Revision = "owner/2"
	sameWithNewRevision, err := DescribeProviderChanges(rv, ru, requests, revision)
	if err != nil {
		t.Fatal(err)
	}
	if sameWithNewRevision.PlanID == plan.PlanID {
		t.Fatal("regression witness: revision churn does not change the digest input (fixture defect)")
	}
	equal, err = ProviderImpactPlansConsentEquivalent(plan, sameWithNewRevision, rv, rv, ru, ru, ro, revision)
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Fatal("a re-derived observation revision alone changed the consent authority")
	}
	dependency := s
	df := dependency.Facts["secrets"]
	df.Dependencies = []string{}
	dependency.Facts["secrets"] = df
	changedDependency, err := DescribeProviderChanges(dependency, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	equal, err = ProviderImpactPlansConsentEquivalent(plan, changedDependency, s, dependency, u, u, o, o)
	if err != nil {
		t.Fatal(err)
	}
	if equal {
		t.Fatal("a dependency change kept the consent authority equal")
	}
	// A real runtime change: the fact AND every witness move together.
	realChange := flipObservation(o, "state")
	rcs := runningSnapshot
	rcf := rcs.Facts["sql"]
	rcf.State = ProviderStoppedOwned
	rcs.Facts["sql"] = rcf
	rco := ProviderImpactObservation{ObservedAt: realChange.ObservedAt, Revision: realChange.Revision, InventoryComplete: realChange.InventoryComplete, Functions: realChange.Functions, ConsumerActions: realChange.ConsumerActions, Continuations: realChange.Continuations, Providers: map[string]ProviderImpactFact{}}
	for id, f := range realChange.Providers {
		c := f
		if id == "sql" {
			c.Realization.State = ProviderStoppedOwned
		}
		rco.Providers[id] = c
	}
	real, err := DescribeProviderChanges(rcs, u, requests, rco)
	if err != nil {
		t.Fatal(err)
	}
	equal, err = ProviderImpactPlansConsentEquivalent(plan, real, s, rcs, u, u, o, rco)
	if err != nil {
		t.Fatal(err)
	}
	if equal {
		t.Fatal("a real runtime state change kept the consent authority equal")
	}
}

func TestConsentSurvivesUnverifiableStateOscillation(t *testing.T) {
	// The snapshot fact carries the classification (Layer-1 default), so both
	// flips must rebuild it together with the observation.
	s, u, o := unverifiedFixture(t)
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" {
		t.Fatal("fixture produced no consent identity")
	}
	consent := ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true}
	if err := ValidateProviderChangeConsent(plan, consent); err != nil {
		t.Fatal(err)
	}

	// C3 contract: a plan-vs-replan pair whose only difference is the
	// classification revalidates equal, and the raw digest must NOT be treated
	// as the staleness authority. The plan-side here is exactly class (ii): the
	// fact classifies the provider unverifiable AND the observation asserts
	// nothing concrete, so the fold leaves it UNKNOWN and UNKNOWN contradicts
	// nothing. The `running` snapshot is an independent deep copy of `s` (the
	// original `s` is reused verbatim for the `back` determinism check below,
	// and DescribeProviderChanges mutates the observation in place), so the
	// state flip cannot leak into the deterministic-input check.
	running := s
	running.Facts = map[string]ProviderFact{}
	for id, f := range s.Facts {
		clone := f
		running.Facts[id] = clone
	}
	rsf := running.Facts["secrets"]
	rsf.State = ProviderRunning
	running.Facts["secrets"] = rsf
	flipped, err := DescribeProviderChanges(running, u, requests, flipObservation(o, "state"))
	if err != nil {
		t.Fatal(err)
	}
	// The displayed digest does change while the classifier flaps (that is the
	// documented oscillation); the consent VALIDATOR keeps its exact plan-id
	// identity, and the post-consent REVALIDATION decides through the consent
	// authority equivalence. This is the C3 contract: a plan-vs-replan pair
	// whose only difference is the classification must revalidate equal, and
	// the raw digest must NOT be treated as the staleness authority.
	if flipped.Complete == plan.Complete {
		t.Fatal("regression witness: the state flip did not change plan knowledge, so the documented oscillation window is not modeled (fixture defect)")
	}
	if flipped.PlanID == plan.PlanID {
		t.Fatal("regression witness: the state flip did not change PlanID, so the documented digest churn is not modeled (fixture defect)")
	}
	if err := ValidateProviderImpactRevalidation(flipped, plan); err != nil {
		var change *ProviderChangeError
		if !errors.As(err, &change) || change.Code != "stale-impact-consent" {
			t.Fatal(err)
		}
		t.Fatalf("A-F1 livelock: transient unverifiable classification of unchanged authority invalidated informed consent: %v", err)
	}

	back, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if back.PlanID != plan.PlanID {
		t.Fatal("planning is not deterministic for identical inputs")
	}
	if err := ValidateProviderImpactRevalidation(back, plan); err != nil {
		t.Fatalf("consent lost when the classification flipped back: %v", err)
	}

	flippedRevision, err := DescribeProviderChanges(running, u, requests, flipObservation(o, "revision"))
	if err != nil {
		t.Fatal(err)
	}
	if flippedRevision.PlanID == plan.PlanID {
		t.Fatal("regression witness: the revision churn did not change PlanID, so the documented digest churn is not modeled (fixture defect)")
	}
	if err := ValidateProviderImpactRevalidation(flippedRevision, plan); err != nil {
		var change *ProviderChangeError
		if !errors.As(err, &change) || change.Code != "stale-impact-consent" {
			t.Fatal(err)
		}
		t.Fatalf("A-F1 livelock: a re-derived observation revision of unchanged authority invalidated informed consent: %v", err)
	}

	// Negative pin (0428-116 home semantics unchanged): genuine underlying
	// state change — here the dependency graph — still invalidates consent.
	genuine := s
	genuine.Facts = map[string]ProviderFact{}
	for id, f := range s.Facts {
		c := f
		c.Dependencies = append([]string(nil), f.Dependencies...)
		if id == "secrets" {
			c.Dependencies = []string{}
		}
		genuine.Facts[id] = c
	}
	changed, err := DescribeProviderChanges(genuine, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if changed.PlanID == plan.PlanID {
		t.Fatal("underlying dependency change did not invalidate consent")
	}
	if err := ValidateProviderChangeConsent(changed, consent); err == nil {
		t.Fatal("stale consent accepted for a genuinely changed plan")
	}
	if err := ValidateProviderImpactRevalidation(changed, plan); err == nil {
		t.Fatal("stale consent accepted for a genuinely changed plan (revalidation)")
	}
}

// Second documented window (B2/observation timing): only the clock sample
// differs between plan and revalidation. Material/validity semantics that
// depend on the sample remain bound through the plan body (expiry boundaries),
// so the identity must not churn on wall-clock time.
func TestConsentSurvivesObservationClockChurn(t *testing.T) {
	s, u, o := unverifiedFixture(t)
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	running := s
	rsf := running.Facts["secrets"]
	rsf.State = ProviderRunning
	running.Facts["secrets"] = rsf
	later := o
	later.ObservedAt = "2026-11-19T06:00:00Z"
	changed, err := DescribeProviderChanges(s, u, requests, later)
	if err != nil {
		t.Fatal(err)
	}
	// Identity boundary (digest.go): the current-time sample is deliberately
	// excluded from consent identity, so no plan-id staleness may arise from
	// resampling alone.
	if err := ValidateProviderImpactRevalidation(changed, plan); err != nil {
		var typed *ProviderChangeError
		if !errors.As(err, &typed) || typed.Code != "stale-impact-consent" {
			t.Fatal(err)
		}
		t.Fatalf("wall-clock sampling alone invalidated informed consent: %v", err)
	}
	// Authority boundary: where a validity window actually crossed at the
	// sampled instant, the plan BODY (outcome/expiry meaning) changes and the
	// consent validity contract must still reject the old acknowledgement.
	vs, vu, vo := impactFixture(t)
	vfact := vo.Providers["secrets"]
	vfact.Materials[0].Kind = "certificate"
	vfact.Materials[0].ValidFrom = "2026-11-19T06:00:00Z"
	vfact.Materials[0].ExpiresAt = "2026-11-20T06:00:00Z"
	vo.Providers["secrets"] = vfact
	vrequests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	notYet := vo
	notYet.ObservedAt = "2026-11-19T05:00:00Z"
	before, err := DescribeProviderChanges(vs, vu, vrequests, notYet)
	if err != nil {
		t.Fatal(err)
	}
	started := vo
	started.ObservedAt = "2026-11-19T07:00:00Z"
	after, err := DescribeProviderChanges(vs, vu, vrequests, started)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProviderChangeConsent(after, ProviderChangeConsent{PlanID: before.PlanID, First: true, Second: true}); err == nil {
		t.Fatal("a crossed validity boundary with only the clock sampled accepted stale consent")
	}
}

// The three regression suites for 0428-119 independent-review befund-2 / C-blockend
// (akten evidence/r28-welle4-2026-11-19/codex-review-wave/1690-0428-119-independent-review.md):
// equalRegistry at provider_consent_authority.go:432 compared only the LENGTHS of
// Instances and Bindings. A provider change that keeps the same number of
// instances/bindings but swaps the instance, retargets the binding or retitles
// the version/readiness of a same-id instance read as consent-equivalent and
// passed the fail-closed gate — exactly the stale-impact-consent hole this card
// exists to close. These suites pin equalRegistry to CONTENT (keyset equality,
// order irrelevant) and pin the end-to-end revalidation gate to fail closed on a
// same-count/different-content registry change.

// registryInstance builds a single-instance registry with a given version/scope.
func registryInstance(id string, version string, scope capability.ProviderScope) capability.Registry {
	r := capability.NewRegistry()
	r.Instances = []capability.ProviderInstance{{
		ID:                 id,
		ProviderID:         "native/" + id,
		ProviderVersion:    version,
		Provider:           capability.Provider{Kind: capability.ProviderOpenBao},
		Scope:              scope,
		Ownership:          capability.OwnershipBaseHarbor,
		OwnerApplicationID: "app",
	}}
	return r
}

// registryWith builds a registry from an ordered list of (id, version, scope),
// so two registries can carry identical content in different order.
func registryWith(t *testing.T, entries ...[3]string) capability.Registry {
	t.Helper()
	r := capability.NewRegistry()
	for _, e := range entries {
		id, version, scopeStr := e[0], e[1], e[2]
		scope := capability.ScopeApplication
		if scopeStr == "shared" {
			scope = capability.ScopeShared
		}
		r.Instances = append(r.Instances, capability.ProviderInstance{
			ID:                 id,
			ProviderID:         "native/" + id,
			ProviderVersion:    version,
			Provider:           capability.Provider{Kind: capability.ProviderOpenBao},
			Scope:              scope,
			Ownership:          capability.OwnershipBaseHarbor,
			OwnerApplicationID: "app",
		})
	}
	return r
}

// TestEqualRegistryPositiveSameContentDifferentOrder asserts the positive path:
// two registries that carry the SAME instances/bindings in a DIFFERENT order are
// content-equivalent, so a reordered registry never triggers a false-positive
// oscillation (the stale-impact-consent fold must be order-independent).
func TestEqualRegistryPositiveSameContentDifferentOrder(t *testing.T) {
	// Identical two-instance content, appended in opposite order.
	a := registryWith(t, [3]string{"secrets", "v1", "application"}, [3]string{"cache", "v1", "application"})
	b := registryWith(t, [3]string{"cache", "v1", "application"}, [3]string{"secrets", "v1", "application"})
	if len(a.Instances) != 2 || len(b.Instances) != 2 {
		t.Fatalf("test setup: expected 2 instances each, got %d and %d", len(a.Instances), len(b.Instances))
	}
	if !equalRegistry(a, b) {
		t.Fatal("same registry content in different order must be equivalent (false-positive oscillation)")
	}
	if !equalRegistry(b, a) {
		t.Fatal("reverse order must also be equivalent (symmetry of the fold)")
	}
}

// TestEqualRegistryBlockendSameCountDifferentContent is the actual befund: two
// registries with the SAME COUNT of instances but DIFFERENT content — the
// instance id swapped — are NOT equivalent, and the following request FAILS
// CLOSED. This is the case the old length-only comparison silently passed.
func TestEqualRegistryBlockendSameCountDifferentContent(t *testing.T) {
	a := registryInstance("secrets", "v1", capability.ScopeApplication)
	// Same count (1 instance), but the instance was swapped for a different id
	// (a different provider): the stale entry must not pass as equivalent.
	b := registryInstance("postgres", "v1", capability.ScopeApplication)
	if equalRegistry(a, b) {
		t.Fatal("same instance count, different instance id: registries must be UNEQUIVALENT")
	}
	// Same id, but the version changed: still a real registry content change.
	c := registryInstance("secrets", "v2", capability.ScopeApplication)
	if equalRegistry(a, c) {
		t.Fatal("same id, different provider_version: registries must be UNEQUIVALENT")
	}
	// Same id, but the scope changed (application -> shared): a real authority
	// change, must be UNEQUIVALENT.
	d := registryInstance("secrets", "v1", capability.ScopeShared)
	if equalRegistry(a, d) {
		t.Fatal("same id, different scope: registries must be UNEQUIVALENT")
	}
}

// TestEqualRegistryBlockendBindingRetarget pins the binding side: same instance
// count, but a binding retargeted to a different provider instance is NOT
// equivalent.
func TestEqualRegistryBlockendBindingRetarget(t *testing.T) {
	base := registryInstance("secrets", "v1", capability.ScopeApplication)
	base.Bindings = []capability.ProviderBinding{
		{ApplicationID: "app", Resource: capability.Resource{Kind: capability.Secrets, Name: "token"}, Environment: "dev", ProviderInstanceID: "secrets"},
	}
	retargeted := registryInstance("secrets", "v1", capability.ScopeApplication)
	retargeted.Bindings = []capability.ProviderBinding{
		{ApplicationID: "app", Resource: capability.Resource{Kind: capability.Secrets, Name: "token"}, Environment: "dev", ProviderInstanceID: "postgres"},
	}
	if equalRegistry(base, retargeted) {
		t.Fatal("same instance count, binding retargeted to another provider instance: must be UNEQUIVALENT")
	}
}

// TestEqualRegistryBoundaryEmpty pins the empty/empty and empty/non-empty
// boundary cases.
func TestEqualRegistryBoundaryEmpty(t *testing.T) {
	empty1 := capability.NewRegistry()
	empty2 := capability.NewRegistry()
	if !equalRegistry(empty1, empty2) {
		t.Fatal("empty vs empty registry must be equivalent")
	}
	nonEmpty := registryInstance("secrets", "v1", capability.ScopeApplication)
	if equalRegistry(empty1, nonEmpty) {
		t.Fatal("empty vs non-empty registry must be UNEQUIVALENT")
	}
	if equalRegistry(nonEmpty, empty1) {
		t.Fatal("non-empty vs empty registry must be UNEQUIVALENT (symmetry)")
	}
}

// TestEqualRegistryFailClosedRevalidation is the end-to-end proof: two plans
// whose only registry difference is a same-count/different-content instance
// swap must NOT read as consent-equivalent, and the post-consent revalidation
// gate must fail closed (stale-impact-consent). The positive order-independent
// path is proven in the two positive suites above.
func TestEqualRegistryFailClosedRevalidation(t *testing.T) {
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	s, u, o := impactFixture(t)
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authority == nil {
		t.Fatal("plan carries no frozen consent surface")
	}

	// Build a second observation whose registry carries a DIFFERENT instance in
	// place of "sql" (the request targets "secrets", which stays intact) — same
	// count (3), different content. Swap sql->redis with a different kind.
	s2 := s
	for i := range s2.Registry.Instances {
		if s2.Registry.Instances[i].ID == "sql" {
			s2.Registry.Instances[i].ID = "redis"
			s2.Registry.Instances[i].ProviderID = "native/redis"
			s2.Registry.Instances[i].Provider.Kind = capability.ProviderValkey
		}
	}
	plan2, err := DescribeProviderChanges(s2, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}

	equivalent, err := ProviderImpactPlansConsentEquivalent(plan, plan2, s, s2, u, u, o, o)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("a same-count/different-content registry swap read as consent-equivalent")
	}
	// The second gate on the revalidation site must also fail closed.
	revalErr := ValidateProviderImpactRevalidation(plan2, plan)
	if revalErr == nil {
		t.Fatal("revalidation second gate accepted a same-count/different-content registry swap as equivalent")
	}
	var typed *ProviderChangeError
	if !errors.As(revalErr, &typed) || typed.Code != "stale-impact-consent" {
		t.Fatalf("expected a stale-impact-consent fail-closed error, got %v", revalErr)
	}
}

// 0428-119 r2 / CODEX Finding 1: sameProviderInstanceAuthority omitted the
// external Reference and OwnerEnvironment fields, so two instances that differ
// ONLY in one of those compare as the same consent authority. These negatives
// pin the fix: the reference and the owner-environment each count as an
// authority-bearing field, and a same-instance swap in either one fails closed.
func TestEqualRegistryBlockendReferenceOnly(t *testing.T) {
	a := registryInstance("secrets", "v1", capability.ScopeApplication)
	// Same id and count, but a different external registration reference:
	// the instance now resolves to a different external registration.
	b := registryInstance("secrets", "v1", capability.ScopeApplication)
	b.Instances[0].Reference = "ext/different-registration"
	if equalRegistry(a, b) {
		t.Fatal("same id/count, different external Reference: registries must be UNEQUIVALENT (stale consent hole)")
	}
}

func TestEqualRegistryBlockendOwnerEnvironmentOnly(t *testing.T) {
	a := registryInstance("secrets", "v1", capability.ScopeApplication)
	// Same id and count, but a different owner-environment constraint:
	// the set of environments allowed to bind the instance changes.
	b := registryInstance("secrets", "v1", capability.ScopeApplication)
	b.Instances[0].OwnerEnvironment = "other-env"
	if equalRegistry(a, b) {
		t.Fatal("same id/count, different OwnerEnvironment: registries must be UNEQUIVALENT (stale consent hole)")
	}
}

// 0428-119 r4 / CODEX r3 Finding P1: sameProviderInstanceAuthority omitted the
// owning manifest name (OwnerApplication), so two instances that differ ONLY in
// that field read as the same consent authority. OwnerApplicationID (the owning
// application's id) and OwnerApplication (the owning application's manifest
// name) are distinct fields. This negative pins the fix: a same-instance swap of
// only OwnerApplication fails closed.
func TestEqualRegistryBlockendOwnerApplicationOnly(t *testing.T) {
	a := registryInstance("secrets", "v1", capability.ScopeApplication)
	// Same id, same OwnerApplicationID, but a different owning manifest name:
	// the instance now resolves to a different owning manifest.
	b := registryInstance("secrets", "v1", capability.ScopeApplication)
	b.Instances[0].OwnerApplication = "other-manifest"
	if equalRegistry(a, b) {
		t.Fatal("same id/OwnerApplicationID, different OwnerApplication: registries must be UNEQUIVALENT (stale consent hole)")
	}
}

func TestEqualRegistryFailClosedReferenceAndEnvironment(t *testing.T) {
	requests := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	s, u, o := impactFixture(t)
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}

	// Swap only the Reference of the "sql" instance (same id/count).
	sRef := s
	for i := range sRef.Registry.Instances {
		if sRef.Registry.Instances[i].ID == "sql" {
			sRef.Registry.Instances[i].Reference = "ext/swapped-reference"
		}
	}
	planRef, err := DescribeProviderChanges(sRef, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := ProviderImpactPlansConsentEquivalent(plan, planRef, s, sRef, u, u, o, o)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("a Reference-only instance change read as consent-equivalent (F1)")
	}
	if err := ValidateProviderImpactRevalidation(planRef, plan); err == nil {
		t.Fatal("revalidation second gate accepted a Reference-only change as equivalent")
	}

	// Swap only the OwnerEnvironment of the "sql" instance (same id/count).
	sEnv := s
	for i := range sEnv.Registry.Instances {
		if sEnv.Registry.Instances[i].ID == "sql" {
			sEnv.Registry.Instances[i].OwnerEnvironment = "other-env"
		}
	}
	planEnv, err := DescribeProviderChanges(sEnv, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	equivalentEnv, err := ProviderImpactPlansConsentEquivalent(plan, planEnv, s, sEnv, u, u, o, o)
	if err != nil {
		t.Fatal(err)
	}
	if equivalentEnv {
		t.Fatal("an OwnerEnvironment-only instance change read as consent-equivalent (F1)")
	}
}

// 0428-119 r2 / CODEX Finding 2: equalProviderCapabilities compared slice
// length and then membership of each left value in the right set. A duplicate
// entry on the left ([sql, sql]) masked a missing/different capability on the
// right ([sql, secrets]) and compared equal. This negative proves the multiset
// fix rejects it.
func TestEqualProviderCapabilitiesRejectsDuplicateMasking(t *testing.T) {
	// Same length (2), every element of a present in {sql, secrets}, but the
	// multiset {sql, sql} does not support secrets: must NOT be equal.
	if equalProviderCapabilities([]capability.Kind{capability.SQL, capability.SQL},
		[]capability.Kind{capability.SQL, capability.Secrets}) {
		t.Fatal("duplicate capability [sql,sql] masked a different second capability [sql,secrets]; equalProviderCapabilities must be a multiset comparison")
	}
	// The converse direction is the same bug: [sql, secrets] vs [sql, sql].
	if equalProviderCapabilities([]capability.Kind{capability.SQL, capability.Secrets},
		[]capability.Kind{capability.SQL, capability.SQL}) {
		t.Fatal("duplicate capability [sql,sql] on the right masked a missing capability; equalProviderCapabilities must be a multiset comparison")
	}
	// Identical multisets still compare equal (order irrelevant).
	if !equalProviderCapabilities([]capability.Kind{capability.SQL, capability.Secrets},
		[]capability.Kind{capability.Secrets, capability.SQL}) {
		t.Fatal("identical capability multisets in different order must be equal")
	}
	// A genuine capability loss must still compare unequal.
	if equalProviderCapabilities([]capability.Kind{capability.SQL, capability.Secrets},
		[]capability.Kind{capability.SQL}) {
		t.Fatal("a lost capability must make the multisets unequal")
	}
}
