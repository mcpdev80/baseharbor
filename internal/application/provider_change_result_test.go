package application

import "testing"

func appliedOutcome(i ProviderImpact) ProviderExecutionOutcome {
	out := ProviderExecutionOutcome{InstanceID: i.InstanceID, Operation: i.Operation, Status: "applied", Verified: true, Evidence: "post-operation runtime inventory", Materials: []MaterialOutcome{}}
	if i.After != nil {
		after := *i.After
		after.Verified = true
		after.Evidence = "actual runtime verification"
		out.After = &after
	}
	for _, m := range i.Materials {
		out.Materials = append(out.Materials, MaterialOutcome{MaterialID: m.Before.ID, Disposition: m.After.Disposition, Location: m.After.Destination, Recovery: m.Before.Recovery, Access: m.After.AccessAfter, Verified: true, Evidence: "actual final material inventory"})
	}
	return out
}
func TestProviderImpactFinalPartialRetryIsActual(t *testing.T) {
	s, u, o := impactFixture(t)
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}, {InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	actual := appliedOutcome(p.Impacts[0])
	result, err := RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Outcomes[1].Status != "pending" {
		t.Fatalf("%+v", result)
	}
	pending, err := PendingProviderChanges(p, result)
	if err != nil || len(pending) != 1 || pending[0].InstanceID != "secrets" {
		t.Fatalf("%+v %v", pending, err)
	}
	finished, err := RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual, appliedOutcome(p.Impacts[1])})
	if err != nil || finished.Status != "complete" {
		t.Fatalf("%+v %v", finished, err)
	}
	if todo, err := PendingProviderChanges(p, finished); err != nil || len(todo) != 0 {
		t.Fatal("verified change replayed")
	}
	// An initial proposal, with Verified=false, cannot be a final receipt.
	actual.After = p.Impacts[0].After
	rejected, err := RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual})
	if err == nil && rejected.Outcomes[0].Status == "applied" {
		t.Fatal("proposal passed as accomplished result")
	}
	actual = appliedOutcome(p.Impacts[0])
	actual.Materials[0].Location = nil
	partial, err := RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual})
	if err != nil || partial.Status == "complete" || partial.Outcomes[0].Materials[0].Location != nil {
		t.Fatalf("lost actual failure/location: %+v %v", partial, err)
	}
}

func TestProviderImpactFinalMustAccountForRecoveryAndActualAccess(t *testing.T) {
	s, u, o := impactFixture(t)
	f := o.Providers["identity"]
	f.Materials[0].Recovery = []ResourceLocation{{Owner: "identity", Target: "local", Kind: "backup", Reference: "backup/verified-owned-realm"}}
	o.Providers["identity"] = f
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	actual := appliedOutcome(p.Impacts[0])
	actual.Materials[0].Recovery = nil
	result, err := RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual})
	if err != nil || result.Status == "complete" {
		t.Fatal("missing recovery material reported preserved")
	}
	actual = appliedOutcome(p.Impacts[0])
	actual.After.Access = &ProviderAccess{Method: "unauthenticated unexpected service"}
	result, err = RecordProviderChangeResult(p, []ProviderExecutionOutcome{actual})
	if err != nil || result.Status == "complete" {
		t.Fatal("unexpected access/security change passed verification")
	}
}
