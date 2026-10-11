package application

import (
	"reflect"
	"testing"
)

func scopeFixture(t *testing.T) (ProviderScopeOwner, []ProviderImpactPlan) {
	t.Helper()
	s, u, o := impactFixture(t)
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	second := p
	second.Target = "other"
	second.PlanID = "second-target-authoritative-plan"
	return ProviderScopeOwner{Owner: "existing/global-registration-owner", Reference: "/protected/external-providers.json", Subject: "company-provider", Revision: "observed-owner-content/1"}, []ProviderImpactPlan{p, second}
}

func TestImpactScopeBindsAllTargetsAndRegistrationOwner(t *testing.T) {
	owner, plans := scopeFixture(t)
	original := append([]ProviderImpactPlan(nil), plans...)
	scope, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plans, original) {
		t.Fatal("composition mutated input target plans")
	}
	if len(scope.Targets) != 2 || scope.ConfirmationsRequired != 2 || scope.Attention != "Achtung" {
		t.Fatalf("scope omitted shared consequences: %+v", scope)
	}
	consent := ProviderChangeConsent{PlanID: scope.PlanID, First: true, Second: true, DeleteData: true}
	if err := ValidateProviderImpactScopeConsent(scope, consent); err != nil {
		t.Fatal(err)
	}
	consent.PlanID = plans[0].PlanID
	if err := ValidateProviderImpactScopeConsent(scope, consent); err == nil {
		t.Fatal("one target approval authorized global removal")
	}
	reversed, err := ComposeProviderImpactScope(owner, []ProviderImpactPlan{plans[1], plans[0]}, true)
	if err != nil || reversed.PlanID != scope.PlanID {
		t.Fatal("target traversal order changed consent")
	}
	owner.Revision = "observed-owner-content/2"
	changed, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil || changed.PlanID == scope.PlanID {
		t.Fatal("changed global registration did not invalidate approval")
	}
	tampered := scope
	tampered.ConfirmationsRequired = 1
	if err := ValidateProviderImpactScopeConsent(tampered, ProviderChangeConsent{PlanID: scope.PlanID, First: true}); err == nil {
		t.Fatal("scope confirmation downgrade accepted")
	}
}

func TestImpactScopeWithoutRuntimeTargetAndUnknownInventory(t *testing.T) {
	owner, _ := scopeFixture(t)
	known, err := ComposeProviderImpactScope(owner, nil, true)
	if err != nil || len(known.Targets) != 0 || !known.Complete {
		t.Fatal("unused global registration invented a runtime Target")
	}
	unknown, err := ComposeProviderImpactScope(owner, nil, false)
	if err != nil || unknown.Complete || unknown.ConfirmationsRequired != 2 {
		t.Fatal("unknown multi-target inventory claimed safe")
	}
	if err := ValidateProviderImpactScopeConsent(unknown, ProviderChangeConsent{PlanID: unknown.PlanID, First: true, Second: true}); err != nil {
		t.Fatal("informed unknown-scope choice vetoed")
	}
	if err := ValidateProviderImpactScopeConsent(unknown, ProviderChangeConsent{PlanID: unknown.PlanID, First: true}); err == nil {
		t.Fatal("unknown scope accepted one acknowledgement")
	}
}

func TestImpactScopeRejectsDuplicatesSecretsAndInvalidTargets(t *testing.T) {
	owner, plans := scopeFixture(t)
	if _, err := ComposeProviderImpactScope(owner, []ProviderImpactPlan{plans[0], plans[0]}, true); err == nil {
		t.Fatal("duplicate target accepted")
	}
	plans[0].Target = ""
	if _, err := ComposeProviderImpactScope(owner, plans, true); err == nil {
		t.Fatal("invented targetless leaf accepted")
	}
	owner.Reference = "https://user:password@example.invalid"
	if _, err := ComposeProviderImpactScope(owner, nil, true); err == nil {
		t.Fatal("credential-bearing owner reference accepted")
	}
}

func TestImpactScopeRequiresActualAllTargetReceipts(t *testing.T) {
	owner, plans := scopeFixture(t)
	scope, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil {
		t.Fatal(err)
	}
	outcome := ProviderScopeOwnerOutcome{Owner: owner, Disposition: "removed", Verified: true, Evidence: "read-back confirms registration entry absent"}
	partial, err := RecordProviderImpactScopeResult(scope, outcome, nil)
	if err != nil || partial.Status == "complete" || len(partial.Targets) != 2 {
		t.Fatal("global removal alone became all-consumer success")
	}
	first, err := RecordProviderChangeResult(plans[0], []ProviderExecutionOutcome{appliedOutcome(plans[0].Impacts[0])})
	if err != nil {
		t.Fatal(err)
	}
	partial, err = RecordProviderImpactScopeResult(scope, outcome, []ProviderTargetChangeResult{{Target: plans[0].Target, Result: first}})
	if err != nil || partial.Status == "complete" {
		t.Fatal("one target result became global success")
	}
	second, err := RecordProviderChangeResult(plans[1], []ProviderExecutionOutcome{appliedOutcome(plans[1].Impacts[0])})
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordProviderImpactScopeResult(scope, outcome, []ProviderTargetChangeResult{{Target: plans[0].Target, Result: first}, {Target: plans[1].Target, Result: second}})
	if err != nil || result.Status != "complete" {
		t.Fatalf("actual receipts rejected: %+v %v", result, err)
	}
	second.PlanID = "stale-target"
	if _, err := RecordProviderImpactScopeResult(scope, outcome, []ProviderTargetChangeResult{{Target: plans[1].Target, Result: second}}); err == nil {
		t.Fatal("stale target result accepted")
	}
	outcome.Owner.Revision = "other-owner"
	if _, err := RecordProviderImpactScopeResult(scope, outcome, nil); err == nil {
		t.Fatal("foreign owner receipt accepted")
	}
}

func TestImpactScopeConsentTracksConsequencesButNotCountdownSamples(t *testing.T) {
	owner, plans := scopeFixture(t)
	sample := int64(120)
	plans[0].Impacts[0].Functions[0].RemainingValiditySeconds = &sample
	scope, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil {
		t.Fatal(err)
	}
	other := int64(119)
	plans[0].Impacts[0].Functions[0].RemainingValiditySeconds = &other
	resampled, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil || resampled.PlanID != scope.PlanID {
		t.Fatal("countdown sampling invalidated unchanged aggregate consent")
	}
	plans[0].Impacts[0].Functions[0].Outcome = "unknown"
	changed, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil || changed.PlanID == scope.PlanID {
		t.Fatal("changed target consequences retained aggregate consent")
	}
	plans[0].Destructive = true
	destructive, err := ComposeProviderImpactScope(owner, plans, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProviderImpactScopeConsent(destructive, ProviderChangeConsent{PlanID: destructive.PlanID, First: true, Second: true}); err == nil {
		t.Fatal("two acknowledgements silently granted material deletion")
	}
}
