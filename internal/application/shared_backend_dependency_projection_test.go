package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestSharedBackendAliasesCountOnePhysicalModuleAndProtectRemainingConsumer(t *testing.T) {
	first, second := New("one", "dev", true, false, false), New("two", "prod", true, false, false)
	registry := capability.NewRegistry()
	for _, m := range []Manifest{first, second} {
		if err := registerReferenceProviders(&registry, m); err != nil {
			t.Fatal(err)
		}
	}
	module := SharedBackendRuntimeDependency("local", capability.ProviderPostgreSQL)
	if module.ID == RuntimeStorageDependencyID("local") {
		t.Fatal("different canonical modules aliased solely by their shared Compose project")
	}
	zero, one := int64(0), int64(1)
	snapshot := ResolutionSnapshot{Target: "local", Registry: registry, Ancillary: []capability.ProviderInstance{module}, DependencyRoots: []string{}, Facts: map[string]ProviderFact{}}
	snapshot.Facts[module.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "runtime/existing-shared-backends-owner", Containers: Quantity{Value: &one, Classification: "measured", Source: "owned native module inventory"}}
	for _, instance := range registry.Instances {
		snapshot.Facts[instance.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{module.ID}, Containers: Quantity{Value: &zero, Classification: "measured", Source: "logical binding to physical module"}}
	}
	first.Services.SQL = false
	plan, err := ResolveProviderPlan(snapshot, []ConsumerIntent{{Key: "one", Active: true, Manifest: first, DependencyRoots: []string{}}, {Key: "two", Active: true, Manifest: second, DependencyRoots: []string{}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(plan.Required, module.ID) {
		t.Fatal("remaining consumer lost physical provider dependency")
	}
	for _, change := range plan.Changes {
		if change.InstanceID == module.ID && (change.Action == "stop" || change.Action == "remove-runtime") {
			t.Fatal("alias release stopped the shared physical owner")
		}
	}
	second.Services.SQL = false
	plan, err = ResolveProviderPlan(snapshot, []ConsumerIntent{{Key: "one", Active: true, Manifest: first, DependencyRoots: []string{}}, {Key: "two", Active: true, Manifest: second, DependencyRoots: []string{}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	stops := 0
	for _, change := range plan.Changes {
		if change.InstanceID == module.ID && change.Action == "stop" {
			stops++
		}
	}
	if stops != 1 {
		t.Fatalf("last-consumer physical retirement count=%d", stops)
	}
}
