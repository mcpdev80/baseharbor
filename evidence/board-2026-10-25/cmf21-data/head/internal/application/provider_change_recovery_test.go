package application

import "testing"

func TestProviderChangeExplicitCompleteDeletionIncludesOwnedRecovery(t *testing.T) {
	s, u, o := impactFixture(t)
	backup := ResourceLocation{Owner: "identity", Target: "local", Kind: "backup", Reference: "backup/realm"}
	fact := o.Providers["identity"]
	fact.Materials[0].Recovery = []ResourceLocation{backup}
	fact.Materials = append(fact.Materials, ProviderMaterial{ID: "realm-backup", Kind: "backup", Ownership: "baseharbor", Location: &backup, Recovery: []ResourceLocation{}, Verified: true, Evidence: "actual owned backup inventory"})
	o.Providers["identity"] = fact
	req := ProviderChangeRequest{InstanceID: "identity", Operation: "destroy", Materials: []MaterialDisposition{{MaterialID: fact.Materials[0].ID, Disposition: "deleted", Supported: true, Verified: true, Evidence: "owned deletion adapter preflight"}, {MaterialID: "realm-backup", Disposition: "deleted", Supported: true, Verified: true, Evidence: "owned backup deletion preflight"}}}
	plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProviderChangeConsent(plan, ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true, DeleteData: true}); err != nil {
		t.Fatal(err)
	}
	actual := appliedOutcome(plan.Impacts[0])
	for i := range actual.Materials {
		actual.Materials[i].Recovery = nil
	}
	result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
	if err != nil || result.Status != "complete" {
		t.Fatalf("explicit owned complete deletion blocked by obsolete backup reference: %+v %v", result, err)
	}
	// With only the database selected for deletion, its backup still must survive.
	req.Materials = req.Materials[:1]
	preserving, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o)
	if err != nil {
		t.Fatal(err)
	}
	actual = appliedOutcome(preserving.Impacts[0])
	for i := range actual.Materials {
		actual.Materials[i].Recovery = nil
	}
	result, err = RecordProviderChangeResult(preserving, []ProviderExecutionOutcome{actual})
	if err != nil || result.Status == "complete" {
		t.Fatal("unselected recovery silently dropped")
	}
}

func TestProviderChangeAggregateBackupTransferUsesFinalRecoveryLocation(t *testing.T) {
	old := ResourceLocation{Owner: "source", Target: "local", Kind: "backup", Reference: "backup/old"}
	destination := ResourceLocation{Owner: "destination", Target: "remote", Kind: "backup", Reference: "backup/new"}
	source := MaterialConsequence{Before: ProviderMaterial{ID: "database", Recovery: []ResourceLocation{old}}}
	backup := MaterialConsequence{Before: ProviderMaterial{ID: "backup", Ownership: "baseharbor", Location: &old}, After: MaterialDisposition{Disposition: "transferred", Destination: &destination}}
	aggregate := []ProviderImpact{{InstanceID: "sql", Materials: []MaterialConsequence{source}}, {InstanceID: "backup-provider", Materials: []MaterialConsequence{backup}}}
	expected := expectedMaterialRecovery(source, aggregate)
	if len(expected) != 1 || expected[0] != destination {
		t.Fatal("aggregate handoff retained obsolete recovery location")
	}
	backup.Before.Ownership = "external"
	aggregate[1].Materials[0] = backup
	expected = expectedMaterialRecovery(source, aggregate)
	if len(expected) != 1 || expected[0] != old {
		t.Fatal("foreign recovery location rewritten without authority")
	}
}
