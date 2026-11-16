package application

import (
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func continuationDependencyFixture(t *testing.T) (ResolutionSnapshot, ProviderPlan, ProviderImpactObservation, ProviderChangeRequest) {
	t.Helper()
	s, u, o := impactFixture(t)
	replacement := capability.ProviderInstance{ID: "identity-replacement", Provider: capability.Keycloak, Scope: capability.ScopeShared, SharingBoundary: "replacement", Ownership: capability.OwnershipBaseHarbor}
	storage := capability.ProviderInstance{ID: "storage", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, SharingBoundary: "storage", Ownership: capability.OwnershipBaseHarbor}
	for _, instance := range []capability.ProviderInstance{replacement, storage} {
		if err := s.Registry.Register(instance); err != nil {
			t.Fatal(err)
		}
		s.Facts[instance.ID] = ProviderFact{Target: s.Target, State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}
		o.Providers[instance.ID] = impactProvider(instance, s.Target, "database")
	}
	fact := s.Facts[replacement.ID]
	fact.Dependencies = []string{"sql"}
	s.Facts[replacement.ID] = fact
	fact = s.Facts["sql"]
	fact.Dependencies = []string{"storage"}
	s.Facts["sql"] = fact
	for _, use := range o.Functions {
		if use.InstanceID == "identity" {
			o.Continuations = append(o.Continuations, ProviderFunctionContinuation{Consumer: use.Consumer, Function: use.Function, From: "identity", To: replacement.ID, Supported: true, Verified: true, Evidence: "actual consumer rebind"})
		}
	}
	return s, u, o, ProviderChangeRequest{InstanceID: "identity", Operation: "replace", ReplacementID: replacement.ID, Selection: "explicit"}
}

func TestContinuationRequiresEntireObservedDependencyGraph(t *testing.T) {
	conditions := []string{"stopped", "new", "unknown-state", "missing-fact", "missing-observation", "unsupported", "unverified", "unknown-dependencies", "foreign-target", "cycle", "unregistered"}
	for _, adapter := range []string{"replacement", "consumer-action"} {
		for _, condition := range conditions {
			t.Run(adapter+"/"+condition, func(t *testing.T) {
				s, u, o, request := continuationDependencyFixture(t)
				if adapter == "consumer-action" {
					action := localAuthAction()
					action.Dependencies = []string{"sql"}
					o.ConsumerActions = []ConsumerTransition{action}
					request.Operation, request.ReplacementID = "stop", ""
					request.ConsumerActions = []string{action.ID}
				}
				ready, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{request}, o)
				if err != nil || !ready.Complete || continuationLogin(ready) != "changed" {
					t.Fatalf("verified dependency fixture is not a continuation: %+v %v", ready, err)
				}
				fact, observed := s.Facts["storage"], o.Providers["storage"]
				switch condition {
				case "stopped":
					fact.State, observed.Realization.State = ProviderStoppedOwned, ProviderStoppedOwned
				case "new":
					fact.State, observed.Realization.State = ProviderNewRequired, ProviderNewRequired
				case "unknown-state":
					fact.State, observed.Realization.State = ProviderUnverifiable, ProviderUnverifiable
				case "unsupported":
					observed.Realization.Supported = false
				case "unverified":
					observed.Realization.Verified, observed.Realization.Evidence = false, ""
				case "unknown-dependencies":
					fact.Dependencies = nil
				case "foreign-target":
					fact.Target, observed.Realization.Target = "other-target", "other-target"
				case "cycle":
					fact.Dependencies = []string{"sql"}
				}
				s.Facts["storage"], o.Providers["storage"] = fact, observed
				switch condition {
				case "missing-fact":
					delete(s.Facts, "storage")
				case "missing-observation":
					delete(o.Providers, "storage")
				case "unregistered":
					instances := []capability.ProviderInstance{}
					for _, instance := range s.Registry.Instances {
						if instance.ID != "storage" {
							instances = append(instances, instance)
						}
					}
					s.Registry.Instances = instances
				}
				before, _ := json.Marshal([]any{s, u, o})
				plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{request}, o)
				if err != nil || plan.Complete || plan.ConfirmationsRequired != 2 || plan.PlanID == ready.PlanID {
					t.Fatalf("invalid dependency graph claimed verified continuation: %+v %v", plan, err)
				}
				want := "unavailable"
				if adapter == "consumer-action" {
					want = "unknown"
				}
				if continuationLogin(plan) != want {
					t.Fatalf("lost dependency still presented working login: %+v", plan.Impacts[0].Functions)
				}
				if err := ValidateProviderChangeConsent(plan, ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true}); err != nil {
					t.Fatal("deliberately acknowledged loss became a permanent veto", err)
				}
				if err := ValidateProviderChangeConsent(plan, ProviderChangeConsent{PlanID: ready.PlanID, First: true, Second: true}); err == nil {
					t.Fatal("previous healthy graph consent authorized changed effects")
				}
				after, _ := json.Marshal([]any{s, u, o})
				if string(before) != string(after) {
					t.Fatal("continuation planning changed authoritative inputs")
				}
			})
		}
	}
}

func TestContinuationDoesNotTreatPlannedResumeAsVerifiedAvailability(t *testing.T) {
	s, u, o, request := continuationDependencyFixture(t)
	fact, observed := s.Facts["storage"], o.Providers["storage"]
	fact.State, observed.Realization.State = ProviderStoppedOwned, ProviderStoppedOwned
	s.Facts["storage"], o.Providers["storage"] = fact, observed
	requests := []ProviderChangeRequest{request, {InstanceID: "storage", Operation: "resume"}}
	plan, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil || plan.Complete || continuationLogin(plan) != "unavailable" {
		t.Fatal("planned resume was treated as executed availability", err)
	}
	fact.State, observed.Realization.State = ProviderRunning, ProviderRunning
	s.Facts["storage"], o.Providers["storage"] = fact, observed
	recovered, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{request}, o)
	if err != nil || !recovered.Complete || continuationLogin(recovered) != "changed" || recovered.PlanID == plan.PlanID {
		t.Fatal("actual verified recovery did not permit fresh continuation", err)
	}
}

func TestContinuationUsesCanonicalAncillaryAndSharedDependencyEvidence(t *testing.T) {
	s, u, o, request := continuationDependencyFixture(t)
	instances := []capability.ProviderInstance{}
	for _, instance := range s.Registry.Instances {
		if instance.ID == "storage" {
			instance.Provider.Capabilities = nil
			s.Ancillary = append(s.Ancillary, instance)
		} else {
			instances = append(instances, instance)
		}
	}
	s.Registry.Instances = instances
	fact := s.Facts["storage"]
	fact.Ancillary, fact.OwnerReference = true, "runtime/canonical-storage"
	s.Facts["storage"] = fact
	observed := o.Providers["storage"]
	observed.Realization.Ancillary, observed.Realization.Owner = true, fact.OwnerReference
	o.Providers["storage"] = observed
	// The same leaf occurs on two branches. Sharing is not a dependency cycle.
	fact = s.Facts[request.ReplacementID]
	fact.Dependencies = []string{"sql", "storage"}
	s.Facts[request.ReplacementID] = fact
	plan, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{request}, o)
	if err != nil || !plan.Complete || continuationLogin(plan) != "changed" {
		t.Fatal("canonical ancillary/shared dependency lost verified continuation", err)
	}
	observed.Realization.Verified, observed.Realization.Evidence = false, ""
	o.Providers["storage"] = observed
	unverified, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{request}, o)
	if err != nil || unverified.Complete || continuationLogin(unverified) != "unavailable" {
		t.Fatal("ancillary owner identity substituted for live verification", err)
	}
}

func continuationLogin(plan ProviderImpactPlan) string {
	for _, impact := range plan.Impacts {
		if impact.InstanceID == "identity" {
			for _, function := range impact.Functions {
				if function.Function == "identity.login" {
					return function.Outcome
				}
			}
		}
	}
	return ""
}
