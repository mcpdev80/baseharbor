package application

import (
	"testing"
)

// TestProviderResultNonContinuationBehaviourPin pins the FAIL-CLOSED semantics
// of the receipt-verification guard in provider_change_result.go (the
// fail-closed ruling, reviewer decision 2026-11-19): a NON-continuation
// operation (continuationRequiresDependencyEvidence == false, A false) with an
// applied receipt that STILL carries a continuation edge the approved plan did
// not verify is an unexpected edge and fails closed.
//
// It is now an explicit fail-closed invariant, not a description of pinned
// CURRENT behaviour: any LATER switch of the non-continuation branch back to a
// lenient `applied` status is an explicit, visible assertion change rather than
// a silent drift.
//
// Two side-by-side operands of the same guard:
//   - non_continuation: a stop impact (A false). An applied receipt that carries
//     an UNPLANNED continuation edge on an instance the approved stop never
//     planned. Under the fail-closed guard this fails with status=failed
//     (actual-disposition-differs). This is the fail-closed behaviour pinned
//     here.
//   - non_continuation_clean: the same stop impact with an applied receipt that
//     carries NO continuation edge. This stays the apply path (status=applied);
//     a non-continuation op with a clean receipt is unaffected by fail closed.
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
		// approved stop planned no dependency graph). Under the fail-closed
		// guard this unexpected edge fails closed; it can no longer apply.
		actual.ContinuationDependencies = []ProviderDependencyEvidence{
			{Realization: ProviderRealization{InstanceID: "ghost", Supported: true, Verified: true, Evidence: "e", State: ProviderRunning}, Dependencies: []string{}},
		}
		result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("non-continuation receipt with unexpected edge failed closed: status=%v code=%v", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
		if result.Outcomes[0].Status != "failed" || result.Outcomes[0].ErrorCode != "actual-disposition-differs" {
			t.Fatalf("non-continuation receipt with an unexpected continuation edge must fail closed with actual-disposition-differs: got status=%q code=%q", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
		}
	})
	t.Run("non_continuation_clean", func(t *testing.T) {
		s, u, o := impactFixture(t)
		plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}}, o)
		if err != nil {
			t.Fatal(err)
		}
		imp := plan.Impacts[0]
		if continuationRequiresDependencyEvidence(imp) {
			t.Fatal("stop impact must not require dependency evidence")
		}
		// A non-continuation op with an applied receipt that carries NO
		// continuation edge is the apply path and is unaffected by fail closed.
		actual := appliedOutcome(imp)
		if len(actual.ContinuationDependencies) != 0 {
			t.Fatalf("clean fixture must carry no continuation edge: got %v", actual.ContinuationDependencies)
		}
		result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("non-continuation clean receipt applied cleanly: status=%v code=%v", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
		if result.Outcomes[0].Status != "applied" {
			t.Fatalf("non-continuation clean receipt must apply: got status=%q code=%q, want applied", result.Outcomes[0].Status, result.Outcomes[0].ErrorCode)
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
		if cdResult.Outcomes[0].Status != "failed" || cdResult.Outcomes[0].ErrorCode != "continuation-dependency-unverified" {
			t.Fatalf("continuation without evidence must fail with continuation-dependency-unverified: got status=%q code=%q", cdResult.Outcomes[0].Status, cdResult.Outcomes[0].ErrorCode)
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
