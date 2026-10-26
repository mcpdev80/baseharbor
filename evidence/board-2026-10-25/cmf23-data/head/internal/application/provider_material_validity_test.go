package application

import (
	"strings"
	"testing"
)

func TestMaterialValidityWindowBindsTransitionsWithoutCountdownChurn(t *testing.T) {
	s, union, observation := impactFixture(t)
	fact := observation.Providers["secrets"]
	fact.Materials[0].Kind = "certificate"
	fact.Materials[0].ValidFrom = "2026-10-12T10:00:00Z"
	fact.Materials[0].ExpiresAt = "2026-10-13T10:00:00Z"
	observation.Providers["secrets"] = fact
	request := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	plan := func(at, meaning string) ProviderImpactPlan {
		t.Helper()
		observation.ObservedAt = at
		value, err := DescribeProviderChanges(s, union, request, observation)
		if err != nil {
			t.Fatal(err)
		}
		material := value.Impacts[0].Materials[0]
		if !strings.Contains(material.Meaning, meaning) || material.After.Disposition != "retained" {
			t.Fatalf("invalid time/retention claim: %#v", material)
		}
		return value
	}
	notYet := plan("2026-10-11T10:00:00Z", "validity has not begun")
	sampled := plan("2026-10-11T10:00:01Z", "validity has not begun")
	if sampled.PlanID != notYet.PlanID {
		t.Fatal("countdown changed approved consequences")
	}
	current := plan("2026-10-12T10:00:00Z", "usable access, trust and revocation status are not implied")
	if current.PlanID == notYet.PlanID {
		t.Fatal("validity start did not invalidate prior consequences")
	}
	expired := plan("2026-10-13T10:00:00Z", "validity has expired")
	if expired.PlanID == current.PlanID {
		t.Fatal("expiry did not invalidate prior consequences")
	}
}

func TestMaterialValidityRejectsInvalidBounds(t *testing.T) {
	for _, start := range []string{"invalid", "2026-10-14T10:00:00Z"} {
		s, union, observation := impactFixture(t)
		fact := observation.Providers["secrets"]
		fact.Materials[0].ValidFrom, fact.Materials[0].ExpiresAt = start, "2026-10-13T10:00:00Z"
		observation.Providers["secrets"] = fact
		if _, err := DescribeProviderChanges(s, union, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, observation); err == nil {
			t.Fatal("invalid validity evidence accepted")
		}
	}
}
