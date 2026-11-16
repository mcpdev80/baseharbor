package application

import "testing"

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
