package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	"testing"
)

func localAuthAction() ConsumerTransition {
	binding := capability.SecureBinding{Credentials: []capability.CredentialReference{{Name: "existing-admin", Reference: "baseharbor://devaccess/local/dev/admin"}}}
	return ConsumerTransition{Outcome: "changed", ID: "console-local-auth", Consumer: "console", InstanceID: "identity", Function: "identity.login", ModeBefore: "oidc", ModeAfter: "local-password", OwnerBefore: "devaccess", OwnerAfter: "devaccess", Before: binding, After: binding, Locations: []ResourceLocation{{Owner: "devaccess", Target: "local", Kind: "path", Reference: "devaccess/local/dev"}}, Dependencies: []string{}, SynchronizationBefore: "verified shared credential users", SynchronizationAfter: "retained values; synchronization unmanaged", RotationBefore: "existing owner", RotationAfter: "manual owner", Supported: true, Verified: true, Evidence: "existing consumer adapter preflight"}
}

func TestProviderChangeConsumerActionExplicitAndFinalVerification(t *testing.T) {
	s, u, o := impactFixture(t)
	action := localAuthAction()
	o.ConsumerActions = []ConsumerTransition{action}
	req := ProviderChangeRequest{InstanceID: "identity", Operation: "stop"}
	unselected, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range unselected.Impacts[0].Functions {
		if f.Function == "identity.login" && (f.Transition != nil || f.Outcome != "unavailable") {
			t.Fatal("invented non-OIDC fallback")
		}
	}
	req.ConsumerActions = []string{action.ID}
	plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o)
	if err != nil {
		t.Fatal(err)
	}
	var selected bool
	for _, f := range plan.Impacts[0].Functions {
		if f.Function == "identity.login" {
			selected = f.Transition != nil && f.Outcome == "changed"
		}
	}
	if !selected || plan.PlanID == unselected.PlanID {
		t.Fatal("explicit consumer reconfiguration missing from concrete consent")
	}
	out := appliedOutcome(plan.Impacts[0])
	result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{out})
	if err != nil || result.Status != "complete" {
		t.Fatalf("%+v %v", result, err)
	}
	for i := range out.Functions {
		if out.Functions[i].Transition != nil {
			copy := *out.Functions[i].Transition
			copy.RotationAfter = "unexpected automatic reset"
			out.Functions[i].Transition = &copy
		}
	}
	result, err = RecordProviderChangeResult(plan, []ProviderExecutionOutcome{out})
	if err != nil || result.Status == "complete" {
		t.Fatal("credential manager diverged but change called complete")
	}
	pending, err := PendingProviderChanges(plan, result)
	if err != nil || len(pending) != 1 || !containsString(pending[0].ConsumerActions, action.ID) {
		t.Fatal("retry lost selected consumer action")
	}
}

func TestProviderChangeCredentialsAndUnverifiedContinuation(t *testing.T) {
	s, u, o := impactFixture(t)
	a := localAuthAction()
	a.After.Credentials = []capability.CredentialReference{{Name: "existing-admin", Reference: "baseharbor://devaccess/new"}}
	o.ConsumerActions = []ConsumerTransition{a}
	req := ProviderChangeRequest{InstanceID: "identity", Operation: "stop", ConsumerActions: []string{a.ID}}
	if _, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o); err == nil {
		t.Fatal("silent credential replacement accepted")
	}
	a.ChangeCredentials = true
	a.Verified = false
	o.ConsumerActions = []ConsumerTransition{a}
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{req}, o)
	if err != nil || p.Complete || p.ConfirmationsRequired != 2 {
		t.Fatal("unsupported/unverified continuation reported safe")
	}
	if err := ValidateProviderChangeConsent(p, ProviderChangeConsent{PlanID: p.PlanID, First: true, Second: true}); err != nil {
		t.Fatal("unknown continuation permanently vetoed")
	}
	a = localAuthAction()
	a.Dependencies = []string{"sql"}
	o.ConsumerActions = []ConsumerTransition{a}
	p, err = DescribeProviderChanges(s, u, []ProviderChangeRequest{req, {InstanceID: "sql", Operation: "stop"}}, o)
	if err != nil || p.Complete {
		t.Fatal("compound removal claimed local-auth continuation despite lost dependency")
	}
}

func TestProviderChangeRemainingValidityUsesAuthoritativeObservation(t *testing.T) {
	s, u, o := impactFixture(t)
	req := []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}}
	p, err := DescribeProviderChanges(s, u, req, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range p.Impacts[0].Functions {
		if f.Function == "identity.validate" && (f.RemainingValiditySeconds == nil || *f.RemainingValiditySeconds != 86400) {
			t.Fatal("remaining validity not accounted")
		}
	}
	o.ObservedAt = "2026-10-13T10:00:00Z"
	expired, err := DescribeProviderChanges(s, u, req, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range expired.Impacts[0].Functions {
		if f.Function == "identity.validate" && (f.Outcome != "unavailable" || f.RemainingValiditySeconds == nil || *f.RemainingValiditySeconds != 0) {
			t.Fatal("expired token treated as usable")
		}
	}
	if expired.PlanID == p.PlanID {
		t.Fatal("expiry change did not invalidate approved effect")
	}
	o.ObservedAt = ""
	unknown, err := DescribeProviderChanges(s, u, req, o)
	if err != nil || unknown.Complete || unknown.ConfirmationsRequired != 2 {
		t.Fatal("unknown current validity treated as verified")
	}
}

func TestProviderChangePKIMaterialValidityAndActualEffects(t *testing.T) {
	s, u, o := impactFixture(t)
	f := o.Providers["secrets"]
	f.Materials[0].Kind = "certificate"
	f.Materials[0].ExpiresAt = "2026-10-13T10:00:00Z"
	o.Providers["secrets"] = f
	plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	material := plan.Impacts[0].Materials[0]
	if material.RemainingValiditySeconds == nil || *material.RemainingValiditySeconds != 172800 || material.After.Disposition != "retained" {
		t.Fatal("certificate validity or secure retention omitted")
	}
	for _, function := range plan.Impacts[0].Functions {
		if function.Function == "pki.renew" && function.Outcome != "unavailable" {
			t.Fatal("certificate retention treated as working renewal")
		}
	}
	actual := appliedOutcome(plan.Impacts[0])
	actual.Effects = nil
	result, err := RecordProviderChangeResult(plan, []ProviderExecutionOutcome{actual})
	if err != nil || result.Status == "complete" {
		t.Fatal("operational guarantees were not verified")
	}
}

func TestProviderChangeActualBackingSQLAttributionAndExplicitOutage(t *testing.T) {
	s, u, o := impactFixture(t)
	backing := capability.ProviderInstance{ID: "openbao-native-sql", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	s.Ancillary = []capability.ProviderInstance{backing}
	s.Facts[backing.ID] = ProviderFact{Ancillary: true, OwnerReference: "installation/openbao-runtime", Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}
	fact := impactProvider(backing, "local", "database")
	fact.Realization.Ancillary = true
	fact.Realization.Owner = "installation/openbao-runtime"
	o.Providers[backing.ID] = fact
	secrets := s.Facts["secrets"]
	secrets.Dependencies = []string{backing.ID}
	s.Facts["secrets"] = secrets
	// Rebuild the existing union with its actual consumer manifests and roots.
	one := New("one", "dev", false, false, true)
	one.ApplicationID = contractAppID
	two := New("two", "dev", false, false, true)
	two.ApplicationID = contractOtherID
	console := New("console", "dev", false, false, false)
	console.ApplicationID = "33333333-3333-4333-8333-333333333333"
	console.Services.Identity = true
	var err error
	u, err = ResolveProviderPlan(s, []ConsumerIntent{{Key: "app/one", Active: true, Manifest: one}, {Key: "app/two", Active: true, Manifest: two}, {Key: "console", Active: true, Manifest: console}}, true)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "sql", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared.Impacts[0].Dependents) != 1 || shared.Impacts[0].Dependents[0] != "identity" {
		t.Fatal("all PostgreSQL instances assumed OpenBao backing")
	}
	private, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: backing.ID, Operation: "remove-runtime"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	impact := private.Impacts[0]
	if len(impact.Dependents) != 1 || impact.Dependents[0] != "secrets" || impact.Before.Owner != "installation/openbao-runtime" {
		t.Fatal("actual OpenBao backing dependency/owner omitted")
	}
	for _, function := range impact.Functions {
		if len(function.Via) != 2 || function.Via[0] != "secrets" || function.Via[1] != backing.ID || function.Outcome != "unavailable" {
			t.Fatal("transitive SQL outage misattributed")
		}
	}
	if len(impact.Functions) != 2 || private.ConfirmationsRequired != 2 {
		t.Fatal("two affected apps or concrete outage warning missing")
	}
	if err := ValidateProviderChangeConsent(private, ProviderChangeConsent{PlanID: private.PlanID, First: true, Second: true}); err != nil {
		t.Fatal("explicitly chosen dependent outage permanently prohibited")
	}
}

func TestProviderChangeSelectedModeDependencyMustBeObserved(t *testing.T) {
	s, u, o := impactFixture(t)
	action := localAuthAction()
	action.Dependencies = []string{"unobserved-owner"}
	o.ConsumerActions = []ConsumerTransition{action}
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop", ConsumerActions: []string{action.ID}}}, o)
	if err != nil || p.Complete || p.ConfirmationsRequired != 2 {
		t.Fatal("selected mode used an invented or unobserved dependency")
	}
}

func TestProviderChangeSelectedModeAccountsForTransitiveDependencies(t *testing.T) {
	for _, condition := range []string{"available", "aggregate-stop", "stopped", "missing", "unknown-closure", "foreign-target", "cycle"} {
		t.Run(condition, func(t *testing.T) {
			s, u, o := impactFixture(t)
			action := localAuthAction()
			action.Dependencies = []string{"secrets"} // The existing secrets provider depends on SQL.
			o.ConsumerActions = []ConsumerTransition{action}
			requests := []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop", ConsumerActions: []string{action.ID}}}
			sql := s.Facts["sql"]
			switch condition {
			case "aggregate-stop":
				requests = append(requests, ProviderChangeRequest{InstanceID: "sql", Operation: "stop"})
			case "stopped":
				sql.State = ProviderStoppedOwned
			case "missing":
				delete(s.Facts, "sql")
				delete(o.Providers, "sql")
			case "unknown-closure":
				sql.Dependencies = nil
			case "foreign-target":
				sql.Target = "other"
			case "cycle":
				sql.Dependencies = []string{"secrets"}
			}
			if condition != "missing" {
				s.Facts["sql"] = sql
				fact := o.Providers["sql"]
				fact.Realization.State, fact.Realization.Target = sql.State, sql.Target
				o.Providers["sql"] = fact
			}
			plan, err := DescribeProviderChanges(s, u, requests, o)
			if err != nil {
				t.Fatal(err)
			}
			selected := false
			for _, impact := range plan.Impacts {
				if impact.InstanceID != "identity" {
					continue
				}
				for _, function := range impact.Functions {
					if function.Transition == nil {
						continue
					}
					selected = true
					if condition == "available" {
						if function.Outcome != "changed" || !plan.Complete {
							t.Fatal("available transitive dependency lost verified continuation")
						}
					} else if function.Outcome != "unknown" || plan.Complete || plan.ConfirmationsRequired != 2 {
						t.Fatalf("unavailable transitive dependency concealed: %+v", function)
					}
				}
			}
			if !selected {
				t.Fatal("selected consumer transition disappeared")
			}
			if err := ValidateProviderChangeConsent(plan, ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true}); err != nil {
				t.Fatal("disclosed uncertainty must not veto the developer's choice", err)
			}
		})
	}
}

func TestProviderChangeCountdownDoesNotInvalidateConcreteConsent(t *testing.T) {
	s, u, o := impactFixture(t)
	fact := o.Providers["secrets"]
	fact.Materials[0].ExpiresAt = "2026-10-12T10:00:00Z"
	o.Providers["secrets"] = fact
	request := []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}, {InstanceID: "secrets", Operation: "stop"}}
	before, err := DescribeProviderChanges(s, u, request, o)
	if err != nil {
		t.Fatal(err)
	}
	o.ObservedAt = "2026-10-11T10:00:01Z"
	later, err := DescribeProviderChanges(s, u, request, o)
	if err != nil {
		t.Fatal(err)
	}
	if before.PlanID != later.PlanID {
		t.Fatal("countdown sampling invalidated unchanged concrete consent")
	}
	for _, impact := range later.Impacts {
		if impact.InstanceID == "secrets" && (impact.Materials[0].RemainingValiditySeconds == nil || *impact.Materials[0].RemainingValiditySeconds != 86399) {
			t.Fatal("digest normalization mutated the displayed validity")
		}
	}
	// Absolute validity is authoritative material state and must invalidate consent.
	fact.Materials[0].ExpiresAt = "2026-10-12T09:00:00Z"
	o.Providers["secrets"] = fact
	changed, err := DescribeProviderChanges(s, u, request, o)
	if err != nil || changed.PlanID == before.PlanID {
		t.Fatal("changed actual expiry reused stale consent")
	}
}

func TestProviderChangeMaterialExpiryInvalidatesConsentWithoutValidationFunction(t *testing.T) {
	s, u, o := impactFixture(t)
	fact := o.Providers["secrets"]
	fact.Materials[0].Kind = "certificate"
	fact.Materials[0].ExpiresAt = "2026-10-11T10:00:01Z"
	o.Providers["secrets"] = fact
	request := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
	before, err := DescribeProviderChanges(s, u, request, o)
	if err != nil {
		t.Fatal(err)
	}
	o.ObservedAt = "2026-10-11T10:00:02Z"
	expired, err := DescribeProviderChanges(s, u, request, o)
	if err != nil || expired.PlanID == before.PlanID {
		t.Fatal("actual certificate expiry reused obsolete consent despite no validation function")
	}
	if expired.Impacts[0].Materials[0].RemainingValiditySeconds == nil || *expired.Impacts[0].Materials[0].RemainingValiditySeconds != 0 {
		t.Fatal("expired material retained positive validity")
	}
}
