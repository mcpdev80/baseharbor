package application

import "testing"

// Missing inventory from an unrelated, unchanged Core/provider is not itself
// permission to retire anything, and it must not veto a fully observed
// non-retiring resume. The affected provider and every destructive path still
// require their authoritative inventory.
func TestImpactCompletenessScopesOnlyVerifiedNonRetiringResume(t *testing.T) {
	snapshot, union, observation := impactFixture(t)
	resume := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "resume"}}
	baseline, err := DescribeProviderChanges(snapshot, union, resume, observation)
	if err != nil || !baseline.Complete {
		t.Fatalf("verified baseline resume unexpectedly incomplete: %+v %v", baseline, err)
	}
	observation.InventoryComplete = false
	delete(observation.Providers, "identity")
	scoped, err := DescribeProviderChanges(snapshot, union, resume, observation)
	if err != nil || !scoped.Complete {
		t.Fatalf("unrelated unchanged provider blocked verified resume: %+v %v", scoped, err)
	}
	stopping, err := DescribeProviderChanges(snapshot, union, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, observation)
	if err != nil || stopping.Complete {
		t.Fatalf("missing global inventory admitted actual stop: %+v %v", stopping, err)
	}
	affected := observation.Providers["secrets"]
	affected.InventoryComplete = false
	observation.Providers["secrets"] = affected
	incomplete, err := DescribeProviderChanges(snapshot, union, resume, observation)
	if err != nil || incomplete.Complete {
		t.Fatalf("missing affected-provider inventory admitted resume: %+v %v", incomplete, err)
	}
}
