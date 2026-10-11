package application

import (
	"errors"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"testing"
)

func TestObservedSharedGatewayUsesSameClosureWithoutCapabilityOffer(t *testing.T) {
	s, u, o := impactFixture(t)
	gateway := capability.ProviderInstance{ID: "native/shared-gateway", Provider: capability.Provider{Kind: capability.ProviderCaddy}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	s.Ancillary = []capability.ProviderInstance{gateway}
	s.Facts[gateway.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "installation/existing-stable-owner"}
	identity := s.Facts["identity"]
	identity.Dependencies = append(identity.Dependencies, gateway.ID)
	s.Facts["identity"] = identity
	owner := New("console", "dev", false, false, false)
	owner.ApplicationID = "33333333-3333-4333-8333-333333333333"
	owner.Services.Identity = true
	u, err := ResolveProviderPlan(s, []ConsumerIntent{{Key: "console", Active: true, Manifest: owner}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !u.Complete || !containsString(u.Required, gateway.ID) {
		t.Fatalf("%+v", u)
	}
	found := false
	for _, fp := range u.Consumers["console"].Existing {
		if fp.InstanceID == gateway.ID {
			found = fp.Ancillary
		}
	}
	if !found {
		t.Fatal("ancillary gateway omitted from shared dependency footprint")
	}
	fact := impactProvider(gateway, "local", "configuration")
	fact.Realization.Owner = s.Facts[gateway.ID].OwnerReference
	fact.Realization.Ancillary = true
	o.Providers[gateway.ID] = fact
	p, err := DescribeProviderChanges(s, u, []ProviderChangeRequest{{InstanceID: gateway.ID, Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Impacts[0].Functions) != 2 || p.Impacts[0].Before.Owner != "installation/existing-stable-owner" || !p.Impacts[0].Before.Ancillary {
		t.Fatalf("%+v", p)
	}
	if gateway.OwnerApplicationID != "" || len(s.Registry.Instances) != 3 {
		t.Fatal("fabricated app ID or registry entry")
	}
	// No registry offer: an HTTP app cannot automatically select the shared
	// observation, and an explicit preference cannot select it either.
	app := WithWorkloadComponents(New("http", "dev", false, false, false), "web")
	app.Exposures = []HTTPExposureRequirement{{Name: "web", Service: "web", Protocol: "http", Port: 8080, Visibility: "public"}}
	fp, err := ResolveFootprint(app, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range fp.Requirements {
		if r.InstanceID == gateway.ID {
			t.Fatal("ancillary selected as a capability offer")
		}
	}
	_, err = ResolveFootprint(app, s, []ProviderPreference{{Requirement: capability.Requirement{Kind: capability.ExposureHTTP, Name: "web"}, InstanceID: gateway.ID}})
	var typed *ResolutionError
	if !errors.As(err, &typed) || typed.Code != "incompatible" {
		t.Fatalf("%v", err)
	}
}

func TestAncillarySidecarOwnershipAndRetirement(t *testing.T) {
	s, _, _ := impactFixture(t)
	executor := capability.ProviderInstance{ID: "native/runtime-executor", Provider: capability.Provider{Kind: capability.ProviderRuntimeExecutor}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: contractAppID, OwnerApplication: "one", OwnerEnvironment: "dev"}
	s.Ancillary = []capability.ProviderInstance{executor}
	s.Facts[executor.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "application/" + contractAppID}
	m := New("one", "dev", false, false, false)
	m.ApplicationID = contractAppID
	u, err := ResolveProviderPlan(s, []ConsumerIntent{{Key: "app/one", Active: true, Manifest: m, DependencyRoots: []string{executor.ID}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !u.Complete || !containsString(u.Required, executor.ID) {
		t.Fatal("sidecar omitted")
	}
	u, err = ResolveProviderPlan(s, []ConsumerIntent{{Key: "app/one", Active: false, Manifest: m}}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range u.Changes {
		if c.InstanceID == executor.ID && c.Action != "stop" {
			t.Fatal("owned last-consumer sidecar did not converge")
		}
	}
	bad := s.Facts[executor.ID]
	bad.OwnerReference = ""
	s.Facts[executor.ID] = bad
	if _, err := ResolveProviderPlan(s, nil, true); err == nil {
		t.Fatal("unproven ancillary owner accepted")
	}
}
