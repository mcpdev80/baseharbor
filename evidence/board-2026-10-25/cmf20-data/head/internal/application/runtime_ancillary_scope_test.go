package application

import (
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestPhysicalExecutorSharedConsumerUnionAndConsequences(t *testing.T) {
	s, _, o := impactFixture(t)
	executor := RuntimeExecutorDependency("local")
	s.Ancillary = []capability.ProviderInstance{executor}
	s.Facts[executor.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Ancillary: true, OwnerReference: "runtime/bh-local-shared/runtime-executor", Dependencies: []string{}}
	observed := impactProvider(executor, "local", "configuration")
	observed.Realization.Ancillary = true
	observed.Realization.Owner = s.Facts[executor.ID].OwnerReference
	o.Providers[executor.ID] = observed
	var consumers []ConsumerIntent
	for index, name := range []string{"one", "two", "console"} {
		m := New(name, "dev", false, false, false)
		m.ApplicationID = []string{contractAppID, "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}[index]
		consumers = append(consumers, ConsumerIntent{Key: name, Active: true, Manifest: m, DependencyRoots: []string{executor.ID}})
		o.Functions = append(o.Functions, ProviderFunctionUse{Consumer: name, InstanceID: executor.ID, Function: "runtime.execute", Required: true, Evidence: "observed executor client configuration"})
	}
	union, err := ResolveProviderPlan(s, consumers, true)
	if err != nil {
		t.Fatal(err)
	}
	if !union.Complete || !reflect.DeepEqual(union.Required, []string{executor.ID}) {
		t.Fatalf("physical executor counted more than once: %+v", union)
	}
	plan, err := DescribeProviderChanges(s, union, []ProviderChangeRequest{{InstanceID: executor.ID, Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Impacts[0].Functions) != 3 || plan.ConfirmationsRequired != 2 {
		t.Fatalf("shared shutdown omitted consumers: %+v", plan)
	}
	for index := range consumers {
		consumers[index].Active = false
		union, err = ResolveProviderPlan(s, consumers, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, change := range union.Changes {
			if change.InstanceID != executor.ID {
				continue
			}
			if index < 2 && change.Action == "stop" {
				t.Fatal("executor stopped while another app/management consumer needs it")
			}
			if index == 2 && (change.Action != "stop" || !change.PreserveData) {
				t.Fatalf("last-consumer removal did not preserve material: %+v", change)
			}
		}
	}
	if RuntimeExecutorDependency("").ID != executor.ID || RuntimeExecutorDependency("other").ID == executor.ID {
		t.Fatal("executor physical identity differs from native project scope")
	}
}

func TestZeroCapabilityWorkloadHasDistinctAncillaryConsequences(t *testing.T) {
	s, _, o := impactFixture(t)
	m := WithWorkloadComponents(New("one", "dev", false, false, false), "web")
	m.ApplicationID = contractAppID
	workload, err := RuntimeWorkloadDependency("local", m, "web")
	if err != nil {
		t.Fatal(err)
	}
	if workload.Provider.Kind == capability.ProviderRuntimeExecutor || len(workload.Provider.Capabilities) != 0 {
		t.Fatal("ordinary workload fabricated privileged executor or Capability offer")
	}
	s.Ancillary = []capability.ProviderInstance{workload}
	s.Facts[workload.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Ancillary: true, OwnerReference: "application/" + m.ApplicationID, Dependencies: []string{}}
	observed := impactProvider(workload, "local", "volume")
	observed.Realization.Ancillary = true
	observed.Realization.Owner = s.Facts[workload.ID].OwnerReference
	o.Providers[workload.ID] = observed
	o.Functions = []ProviderFunctionUse{{Consumer: "one", InstanceID: workload.ID, Function: "workload.run", Required: true, Evidence: "declared and observed component"}}
	union, err := ResolveProviderPlan(s, []ConsumerIntent{{Key: "one", Active: true, Manifest: m, DependencyRoots: []string{workload.ID}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !union.Complete || !reflect.DeepEqual(union.Required, []string{workload.ID}) {
		t.Fatal("zero-capability workload acquired unrelated Core")
	}
	plan, err := DescribeProviderChanges(s, union, []ProviderChangeRequest{{InstanceID: workload.ID, Operation: "stop"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConfirmationsRequired != 2 || len(plan.Impacts[0].Functions) != 1 || plan.Impacts[0].Functions[0].Function != "workload.run" {
		t.Fatalf("workload outage consequences omitted: %+v", plan)
	}
	if _, err := RuntimeWorkloadDependency("local", m, "undeclared"); err == nil {
		t.Fatal("same-project undeclared component acquired ownership")
	}
	m.ApplicationID = ""
	if _, err := RuntimeWorkloadDependency("local", m, "web"); err == nil {
		t.Fatal("workload allocated or manufactured missing identity")
	}
}

func TestAncillaryNativeScopesRejectFalseOwners(t *testing.T) {
	for _, kind := range []capability.ProviderKind{capability.ProviderRuntimeExecutor, capability.ProviderRuntimeBroker, capability.ProviderRuntimeWorkload} {
		s, _, _ := impactFixture(t)
		instance := capability.ProviderInstance{ID: "runtime/wrong-owner", Provider: capability.Provider{Kind: kind}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
		if kind == capability.ProviderRuntimeExecutor {
			instance.Scope = capability.ScopeApplication
			instance.OwnerApplicationID = contractAppID
			instance.OwnerApplication = "one"
		}
		s.Ancillary = []capability.ProviderInstance{instance}
		s.Facts[instance.ID] = ProviderFact{Target: "local", Ancillary: true, OwnerReference: "runtime/wrong-owner", Dependencies: []string{}}
		if _, err := ResolveProviderPlan(s, nil, true); err == nil {
			t.Fatalf("%s accepted wrong physical scope", kind)
		}
	}
}
