package application

import (
	"testing"
)

// TestProviderResultNonContinuationBehaviourPin pins the CURRENT semantics of
// the receipt-verification guard in provider_change_result.go:47 (the
// parenthesised form `continuationRequiresDependencyEvidence(imp) &&
// (len(imp.ContinuationDependencies) == 0 || !continuationDependencyOutcomesMatch(...))`,
// already accepted in fix-1). It is a behaviour-pin, NOT a fail-closed
// invariant: it records what the correct code does TODAY so that any LATER
// switch of the non-continuation branch to a fail-closed `unexpected edge`
// status is an explicit, visible assertion change rather than a silent drift.
//
// Two side-by-side operands of the same guard:
//   - non_continuation: a stop impact (continuationRequiresDependencyEvidence
//     == false, A false). An applied receipt that carries an UNPLANNED
//     continuation edge on an instance the approved stop never planned. Under
//     the CORRECT guard the guard is false for A==false and the receipt
//     applies cleanly (status=applied). The stray edge is irrelevant to a
//     non-continuation op. This is the behaviour the current code is pinned to.
//     (Under the pre-fix-1 collapsed guard `(A && len==0) || !match` this same
//     case fired on its second operand and was mislabelled
//     `continuation-dependency-unverified`; the pin therefore still catches a
//     regression of the precedence fix, but its stated intent is the pinned
//     CURRENT behaviour, not the fix itself.)
//   - continuation_without_evidence: a replacement impact that requires a
//     dependency graph (A true) with NO evidence supplied. The guard must
//     block it as `continuation-dependency-unverified`. This pins the
//     continuation operand independently of the non-continuation one.
func TestProviderResultNonContinuationBehaviourPin(t *testing.T) {
	t.Run("non_continuation", func(t *testing.T) {
		s, u, o := impactFixture(t)
		// A stop impact: no replacement After, no transition with dependencies,
		// so continuationRequiresDependencyEvidence is false.
		plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}}, o)
		if err != nil {
			t.Fatal(err)
		}
		imp := plan.Impacts[0]
		if continuationRequiresDependencyEvidence(imp) {
			t.Fatal("stop impact must not require dependency evidence")
		}
		actual := appliedOutcome(imp)
		// The applied receipt carries an UNPLANNED continuation edge (the
		// approved stop planned no dependency graph). Under the correct guard
		// the guard cannot fire when A is false, so the receipt applies
		// cleanly — the pinned CURRENT semantics.
		actual.ContinuationDependencies = []ProviderDependencyEvidence{
			{Realization: ProviderRealization{InstanceID: "ghost", Supported: true, Verified: true, Evidence: "e", State: ProviderRunning}, Dependencies: []string{}},
		}
		result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("non-continuation receipt applied cleanly: status=%v code=%v", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
		if result.Outcomes[0].Status != "applied" {
			t.Fatalf("non-continuation receipt must apply under current semantics: got status=%q code=%q, want applied", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
		}
	})
	t.Run("continuation_without_evidence", func(t *testing.T) {
		cdS, cdU, cdO, cdReq := continuationDependencyFixture(t)
		cdPlan, err := DescribeProviderChanges(cdS, cdU, []ProviderChangeRequest{cdReq}, cdO)
		if err != nil || !cdPlan.Complete {
			t.Fatalf("fixture: %+v %v", cdPlan, err)
		}
		cdActual := appliedOutcome(cdPlan.Impacts[0])
		cdActual.ContinuationDependencies = nil // no evidence supplied
		cdResult, err := RecordProviderChangeResult(cdPlan, []ProviderExecutionOutcome{cdActual})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("continuation without evidence blocked: status=%v code=%v", cdResult.Outcomes[0].Status, cdResult.Outcomes[0].ErrorCode)
		if cdResult.Outcomes[0].ErrorCode != "continuation-dependency-unverified" {
			t.Fatalf("continuation without evidence must be blocked: got status=%q code=%q, want continuation-dependency-unverified", cdResult.Outcomes[0].Status, cdResult.Outcomes[0].ErrorCode)
		}
	})
}

func TestProviderResultRequiresFreshEntireContinuationGraph(t *testing.T) {
	for _, mode := range []string{"replacement", "consumer-action"} {
		for _, condition := range []string{"valid", "absent-receipt", "legacy-plan", "stopped", "unverified", "unknown-dependencies", "new-edge", "duplicate", "wrong-owner", "foreign-target"} {
			t.Run(mode+"/"+condition, func(t *testing.T) {
				s, union, observed, request := continuationDependencyFixture(t)
				if mode == "consumer-action" {
					action := localAuthAction()
					action.Dependencies = []string{"sql"}
					observed.ConsumerActions = []ConsumerTransition{action}
					request.ReplacementID, request.Operation = "", "stop"
					request.ConsumerActions = []string{action.ID}
				}
				plan, err := DescribeProviderChanges(s, union, []ProviderChangeRequest{request}, observed)
				if err != nil || !plan.Complete {
					t.Fatalf("fixture: %+v %v", plan, err)
				}
				if len(plan.Impacts[0].ContinuationDependencies) < 2 {
					t.Fatal("transitive graph omitted from approval")
				}
				actual := appliedOutcome(plan.Impacts[0])
				// Collect after-operation evidence from existing owners, not from
				// proposed After fields. Mutate one actual prerequisite afterwards.
				fact := observed.Providers["storage"]
				source := s.Facts["storage"]
				switch condition {
				case "stopped":
					fact.Realization.State = ProviderStoppedOwned
					source.State = ProviderStoppedOwned
				case "unverified":
					fact.Realization.Verified = false
				case "unknown-dependencies":
					source.Dependencies = nil
				case "new-edge":
					source.Dependencies = []string{"sql"}
				case "wrong-owner":
					fact.Realization.Owner = "foreign-owner"
				case "foreign-target":
					fact.Realization.Target = "other-target"
				}
				observed.Providers["storage"], s.Facts["storage"] = fact, source
				actual.ContinuationDependencies, err = ObserveProviderContinuationDependencies(plan.Impacts[0], s, observed)
				if err != nil {
					if condition == "wrong-owner" || condition == "foreign-target" {
						return
					}
					t.Fatal(err)
				}
				if condition == "legacy-plan" {
					plan.Impacts[0].ContinuationDependencies = nil
					actual.ContinuationDependencies = nil
				}
				if condition == "absent-receipt" {
					actual.ContinuationDependencies = nil
				}
				if condition == "duplicate" {
					actual.ContinuationDependencies[0] = actual.ContinuationDependencies[1]
				}
				result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
				if err != nil {
					t.Fatal(err)
				}
				if condition == "valid" {
					if result.Status != "complete" {
						t.Fatalf("fresh graph rejected: %+v", result)
					}
					return
				}
				if result.Status == "complete" || result.Outcomes[0].ErrorCode != "continuation-dependency-unverified" {
					t.Fatalf("stale plan proof became actual continuation: %+v", result)
				}
				pending, err := PendingProviderChanges(plan, result)
				if err != nil || len(pending) != 1 || pending[0].ReplacementID != request.ReplacementID || mode == "consumer-action" && !containsString(pending[0].ConsumerActions, request.ConsumerActions[0]) {
					t.Fatal("retry lost requested continuation", err)
				}
			})
		}
	}
}
