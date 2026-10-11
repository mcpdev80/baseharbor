package application

import (
	"encoding/json"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"reflect"
	"testing"
)

func impactProvider(i capability.ProviderInstance, target string, kind string) ProviderImpactFact {
	loc := ResourceLocation{Owner: i.ID, Target: target, Kind: "volume", Reference: "volume/" + i.ID}
	return ProviderImpactFact{Realization: ProviderRealization{InstanceID: i.ID, Kind: i.Provider.Kind, Target: target, Scope: i.Scope, Ownership: i.Ownership, Owner: i.OwnerApplicationID, State: ProviderRunning, Locations: []ResourceLocation{loc}, Supported: true, Verified: true, Evidence: "verified runtime inventory"},
		Materials: []ProviderMaterial{{ID: "data/" + i.ID, Kind: kind, Ownership: i.Ownership, Location: &loc, Recovery: []ResourceLocation{}, Verified: true, Evidence: "verified volume labels and inventory"}}, InventoryComplete: true,
		Effects: []OperationalConsequence{{Aspect: "availability", Before: "running", After: "service unavailable until resume or verified replacement", Evidence: "runtime operation"}}}
}

func impactFixture(t *testing.T) (ResolutionSnapshot, ProviderPlan, ProviderImpactObservation) {
	t.Helper()
	r := capability.NewRegistry()
	for _, i := range []capability.ProviderInstance{
		{ID: "sql", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "identity", Provider: capability.Keycloak, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "secrets", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	} {
		if err := r.Register(i); err != nil {
			t.Fatal(err)
		}
	}
	s := ResolutionSnapshot{Target: "local", Registry: r, Facts: map[string]ProviderFact{}}
	o := ProviderImpactObservation{Revision: "owner/1", InventoryComplete: true, Providers: map[string]ProviderImpactFact{}}
	for _, i := range r.Instances {
		s.Facts[i.ID] = ProviderFact{Target: s.Target, State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}
		materialKind := "database"
		if i.ID == "secrets" {
			materialKind = "secret"
		}
		if i.ID == "identity" {
			materialKind = "realm"
		}
		o.Providers[i.ID] = impactProvider(i, s.Target, materialKind)
	}
	for _, id := range []string{"identity", "secrets"} {
		f := s.Facts[id]
		f.Dependencies = []string{"sql"}
		s.Facts[id] = f
	}
	one := New("one", "dev", false, false, true)
	one.ApplicationID = contractAppID
	two := New("two", "dev", false, false, true)
	two.ApplicationID = contractOtherID
	console := New("console", "dev", false, false, false)
	console.ApplicationID = "33333333-3333-4333-8333-333333333333"
	console.Services.Identity = true
	consumers := []ConsumerIntent{{Key: "app/one", Active: true, Manifest: one}, {Key: "app/two", Active: true, Manifest: two}, {Key: "console", Active: true, Manifest: console}}
	p, err := ResolveProviderPlan(s, consumers, true)
	if err != nil {
		t.Fatal(err)
	}
	o.Functions = []ProviderFunctionUse{
		{Consumer: "app/one", InstanceID: "secrets", Function: "secrets.delivery", Required: true, Evidence: "applied secret binding"},
		{Consumer: "app/two", InstanceID: "secrets", Function: "pki.renew", Required: true, Evidence: "applied certificate issuer"},
		{Consumer: "console", InstanceID: "identity", Function: "identity.login", Required: true, Evidence: "management OIDC configuration"},
		{Consumer: "console", InstanceID: "identity", Function: "identity.validate", Required: true, ValidUntil: "2026-10-12T10:00:00Z", Behavior: "offline-until-expiry", Evidence: "verified local validation and token expiry"},
	}
	return s, p, o
}

func TestProviderImpactTransitiveAggregateAndSharedConsumers(t *testing.T) {
	s, u, o := impactFixture(t)
	before, _ := json.Marshal(struct {
		S ResolutionSnapshot
		U ProviderPlan
		O ProviderImpactObservation
	}{s, u, o})
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "sql", Operation: "stop"}, {InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Complete || p.Attention != "Achtung" || p.ConfirmationsRequired != 2 || len(p.Impacts) != 2 {
		t.Fatalf("%+v", p)
	}
	sql := p.Impacts[1]
	if !reflect.DeepEqual(sql.Dependents, []string{"identity", "secrets"}) || len(sql.Functions) != 4 {
		t.Fatalf("transitive impact: %+v", sql)
	}
	for _, f := range sql.Functions {
		if len(f.Via) != 2 {
			t.Fatalf("missing dependency path: %+v", f)
		}
	}
	after, _ := json.Marshal(struct {
		S ResolutionSnapshot
		U ProviderPlan
		O ProviderImpactObservation
	}{s, u, o})
	if string(before) != string(after) {
		t.Fatal("planning mutated authoritative inputs")
	}
	reordered, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}, {InstanceID: "sql", Operation: "stop"}}, o)
	if err != nil || reordered.PlanID != p.PlanID {
		t.Fatal("request order changed acknowledgement")
	}
}

func TestProviderImpactNoContinuationAllowedAfterTwoConfirmations(t *testing.T) {
	s, u, o := impactFixture(t)
	o.InventoryComplete = false
	delete(o.Providers, "secrets")
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Complete || p.ConfirmationsRequired != 2 {
		t.Fatalf("%+v", p)
	}
	if err := ValidateProviderChangeConsent(p, ProviderChangeConsent{PlanID: p.PlanID, First: true, Second: true}); err != nil {
		t.Fatal("developer choice vetoed:", err)
	}
	// Actual PKI use is shown, but unused dynamic credentials are not invented.
	for _, f := range p.Impacts[0].Functions {
		if f.Function == "credentials.issue" {
			t.Fatal("invented OpenBao use")
		}
	}
}

func TestProviderImpactMaterialDeletionRetentionAndSecureTransfer(t *testing.T) {
	s, u, o := impactFixture(t)
	m := o.Providers["secrets"].Materials[0]
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "destroy", Materials: []MaterialDisposition{{MaterialID: m.ID, Disposition: "deleted"}}}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Destructive || p.ConfirmationsRequired != 2 || p.Impacts[0].Materials[0].After.Destination != nil || p.Impacts[0].Materials[0].Meaning != "no destination; deletion" {
		t.Fatalf("%+v", p)
	}
	if err := ValidateProviderChangeConsent(p, ProviderChangeConsent{PlanID: p.PlanID, First: true, Second: true, DeleteData: true}); err != nil {
		t.Fatal(err)
	}
	retained, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil || retained.Impacts[0].Materials[0].After.AccessAfter != nil {
		t.Fatal("retained volume reported as live delivery")
	}
	dest := ResourceLocation{Owner: "owned-secure-destination", Target: "local", Kind: "external-store", Reference: "authorized-store/2"}
	transfer := MaterialDisposition{MaterialID: m.ID, Disposition: "transferred", Destination: &dest, Supported: false}
	incomplete, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop", Materials: []MaterialDisposition{transfer}}}, o)
	if err != nil || incomplete.Complete || incomplete.ConfirmationsRequired != 2 {
		t.Fatalf("%+v %v", incomplete, err)
	}
	transfer.Supported = true
	transfer.Verified = true
	transfer.Evidence = "secure handoff verification receipt"
	transfer.AccessAfter = &ProviderAccess{Method: "verified secure provider delivery", Security: capability.SecureBinding{Secrets: []capability.SecretReference{{Name: "api", Reference: "baseharbor://secrets/receiver/api"}}}}
	secure, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop", Materials: []MaterialDisposition{transfer}}}, o)
	if err != nil || !secure.Complete {
		t.Fatalf("%+v %v", secure, err)
	}
}

func TestProviderImpactStaleApprovalAndOwnership(t *testing.T) {
	s, u, o := impactFixture(t)
	requests := []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}}
	p, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	old := ProviderChangeConsent{PlanID: p.PlanID, First: true, Second: true}
	o.Revision = "owner/2"
	next, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	var typed *ProviderChangeError
	if err := ValidateProviderChangeConsent(next, old); !errors.As(err, &typed) || typed.Code != "stale-impact-consent" {
		t.Fatal(err)
	}
	f := o.Providers["identity"]
	f.Realization.Ownership = capability.OwnershipExternal
	o.Providers["identity"] = f
	if _, err := DescribeProviderChanges(s, u, requests, o); err == nil {
		t.Fatal("observer changed authoritative ownership")
	}
}

func TestProviderImpactTokensCertificatesAndDynamicLeases(t *testing.T) {
	s, u, o := impactFixture(t)
	o.Functions = append(o.Functions, ProviderFunctionUse{Consumer: "app/one", InstanceID: "secrets", Function: "credentials.renew", Required: true, ValidUntil: "2026-10-12T10:00:00Z", Behavior: "downstream-validity-unverified", Evidence: "actual dynamic lease"})
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "identity", Operation: "stop"}, {InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range p.Impacts {
		for _, f := range i.Functions {
			if f.Function == "identity.validate" && (f.Outcome != "changed" || f.ValidUntil == "") {
				t.Fatal("uniform instant token invalidation")
			}
			if f.Function == "credentials.renew" && f.Outcome != "unavailable" {
				t.Fatal("dynamic renewal presumed valid")
			}
		}
	}
}

func TestProviderImpactEveryRegisteredProviderAndExternalOwnership(t *testing.T) {
	for _, d := range capability.ReferenceIntegrations() {
		t.Run(string(d.Provider.Kind), func(t *testing.T) {
			scope := d.SupportedScopes[0]
			ownership := capability.OwnershipBaseHarbor
			if scope == capability.ScopeExternal {
				ownership = capability.OwnershipExternal
			}
			i := capability.ProviderInstance{ID: string(d.Provider.Kind), Provider: d.Provider, Scope: scope, Ownership: ownership}
			if scope == capability.ScopeApplication {
				i.OwnerApplicationID = contractAppID
				i.OwnerApplication = "app"
				i.OwnerEnvironment = "dev"
			}
			if scope == capability.ScopeExternal {
				i.Reference = "provider-registration"
			}
			r := capability.NewRegistry()
			if err := r.Register(i); err != nil {
				t.Fatal(err)
			}
			s := ResolutionSnapshot{Target: "local", Registry: r, Facts: map[string]ProviderFact{i.ID: {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}}}
			u, err := ResolveProviderPlan(s, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := d.ChangeContract()
			o := ProviderImpactObservation{Revision: "owner/1", InventoryComplete: true, Providers: map[string]ProviderImpactFact{i.ID: impactProvider(i, "local", c.MaterialKinds[0])}, Functions: []ProviderFunctionUse{{Consumer: "app", InstanceID: i.ID, Function: c.Functions[0], Required: true, Evidence: "actual applied binding"}}}
			op := "stop"
			if scope == capability.ScopeExternal {
				op = "unbind"
			}
			p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: i.ID, Operation: op}}, o)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Impacts[0].Functions) != 1 || p.Impacts[0].Before.Kind != d.Provider.Kind || p.Impacts[0].Materials[0].Before.Location == nil || p.ConfirmationsRequired != 2 {
				t.Fatalf("%+v", p)
			}
			if scope == capability.ScopeExternal {
				if _, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: i.ID, Operation: "destroy"}}, o); err == nil {
					t.Fatal("foreign runtime destruction authorized")
				}
			}
		})
	}
}

func TestProviderImpactSecretReferencesNeverLeak(t *testing.T) {
	s, u, o := impactFixture(t)
	f := o.Providers["secrets"]
	f.Realization.Access = &ProviderAccess{Method: "https", Endpoint: "https://user:secret@example.test"}
	o.Providers["secrets"] = f
	if _, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o); err == nil || containsString([]string{err.Error()}, "secret") {
		t.Fatal("unsafe endpoint accepted or echoed")
	}
}

func TestProviderImpactReplacementNeedsActualRebindAndAggregateDependencies(t *testing.T) {
	s, u, o := impactFixture(t)
	replacement := capability.ProviderInstance{ID: "identity-replacement", Provider: capability.Keycloak, Scope: capability.ScopeShared, SharingBoundary: "replacement", Ownership: capability.OwnershipBaseHarbor}
	if err := s.Registry.Register(replacement); err != nil {
		t.Fatal(err)
	}
	s.Facts[replacement.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{"sql"}}
	o.Providers[replacement.ID] = impactProvider(replacement, "local", "realm")
	requests := []ProviderChangeRequest{{InstanceID: "identity", Operation: "replace", ReplacementID: replacement.ID, Selection: "explicit"}}
	unverified, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if unverified.Impacts[0].Functions[0].Outcome != "unavailable" || unverified.ConfirmationsRequired != 2 {
		t.Fatal("running replacement assumed consumer rebind")
	}
	for _, use := range o.Functions {
		if use.InstanceID == "identity" {
			o.Continuations = append(o.Continuations, ProviderFunctionContinuation{Consumer: use.Consumer, Function: use.Function, From: "identity", To: replacement.ID, Supported: true, Verified: true, Evidence: "verified management OIDC rebind"})
		}
	}
	verified, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Impacts[0].After.InstanceID != replacement.ID || verified.Impacts[0].Functions[0].Outcome != "changed" || verified.Impacts[0].Selection != "explicit" {
		t.Fatalf("%+v", verified)
	}
	// Removing the replacement's SQL dependency in the same aggregate invalidates
	// continuation even though its own endpoint was verified before this change.
	requests = append(requests, ProviderChangeRequest{InstanceID: "sql", Operation: "stop"})
	aggregate, err := DescribeProviderChanges(s, u, requests, o)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Impacts[0].Functions[0].Outcome != "unavailable" || aggregate.ConfirmationsRequired != 2 {
		t.Fatal("aggregate invalidated dependency ignored")
	}
}

func TestProviderImpactScopedUnbindRetainsOtherAppAndRetryScope(t *testing.T) {
	s, u, o := impactFixture(t)
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "unbind", Consumers: []string{"app/one"}}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Impacts[0].Scenario != "provider-still-used" || len(p.Impacts[0].Functions) != 1 || p.Impacts[0].Functions[0].Consumer != "app/one" {
		t.Fatalf("%+v", p)
	}
	result, err := RecordProviderChangeResult(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := PendingProviderChanges(p, result)
	if err != nil || len(retry) != 1 || !reflect.DeepEqual(retry[0].Consumers, []string{"app/one"}) {
		t.Fatalf("%+v %v", retry, err)
	}
}

func TestProviderImpactUnpersistedJobsSessionsAndMissingMetadataAreUnknown(t *testing.T) {
	s, u, o := impactFixture(t)
	f := o.Providers["secrets"]
	f.Materials[0].Location = nil
	f.Materials[0].Kind = "session"
	o.Providers["secrets"] = f
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Complete || p.Impacts[0].Materials[0].After.Disposition != "unknown" || p.ConfirmationsRequired != 2 {
		t.Fatal("session assumed disposable")
	}
	// No provider-specific usage contract means unknown, not safe.
	o.Functions = append(o.Functions, ProviderFunctionUse{Consumer: "app/one", InstanceID: "secrets", Function: "undeclared.function", Required: true, Evidence: "adapter custom use"})
	p, err = DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil || p.Complete {
		t.Fatal("undeclared semantics reported known")
	}
}

func TestProviderImpactIntentRemovalAndSamplingDoNotInventDemandOrStaleConsent(t *testing.T) {
	s, u, o := impactFixture(t)
	u.ConsumerActivity["app/one"] = false
	delete(u.Consumers, "app/one")
	u.ConsumerActivity["app/two"] = false
	delete(u.Consumers, "app/two")
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range p.Impacts[0].Functions {
		if f.Required {
			t.Fatal("inactive consumer still requires function")
		}
	}
	if p.Impacts[0].Scenario != "capability-no-longer-requested" {
		t.Fatal(p.Impacts[0].Scenario)
	}
	bytes := int64(4096)
	fact := s.Facts["secrets"]
	fact.MemoryBytes = Quantity{Value: &bytes, Classification: "measured", Source: "new sample"}
	s.Facts["secrets"] = fact
	next, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil || next.PlanID != p.PlanID {
		t.Fatal("incidental sampling invalidated identical effects")
	}
	fact.Dependencies = []string{}
	s.Facts["secrets"] = fact
	changed, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}, o)
	if err != nil || changed.PlanID == p.PlanID {
		t.Fatal("dependency change did not invalidate consent")
	}
}

func TestProviderImpactSecureTransferNeedsDeliveryReference(t *testing.T) {
	s, u, o := impactFixture(t)
	m := o.Providers["secrets"].Materials[0]
	dest := ResourceLocation{Owner: "owned-existing-store", Target: "local", Kind: "external-store", Reference: "authorized-store/1"}
	action := MaterialDisposition{MaterialID: m.ID, Disposition: "transferred", Destination: &dest, AccessAfter: &ProviderAccess{Method: "unverified plaintext fallback"}, Supported: true, Verified: true, Evidence: "not proof of secure delivery"}
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop", Materials: []MaterialDisposition{action}}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Complete || p.ConfirmationsRequired != 2 {
		t.Fatal("method label presumed secure delivery")
	}
	action.Evidence = "https://user:password@example.test"
	if _, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop", Materials: []MaterialDisposition{action}}}, o); err == nil {
		t.Fatal("credential-bearing evidence leaked")
	}
}
