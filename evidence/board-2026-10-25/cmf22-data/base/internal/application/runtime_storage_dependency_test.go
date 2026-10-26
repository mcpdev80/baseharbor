package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestBrokerStorageIsDependencyWithoutAutomaticSQLOffer(t *testing.T) {
	m := New("secrets", "dev", false, false, true)
	r := capability.NewRegistry()
	if err := registerReferenceProviders(&r, m); err != nil {
		t.Fatal(err)
	}
	storage := RuntimeStorageDependency("local")
	for _, i := range r.Instances {
		if i.Provider.Kind == capability.ProviderPostgreSQL || i.ID == storage.ID {
			t.Fatal("private broker backing storage became a selectable SQL offer")
		}
	}
	s := ResolutionSnapshot{Target: "local", Registry: r, Ancillary: []capability.ProviderInstance{storage}, DependencyRoots: []string{}, Facts: map[string]ProviderFact{}}
	s.Facts[storage.ID] = ProviderFact{Target: "local", Ancillary: true, OwnerReference: "runtime/bh-local-shared", State: ProviderNewRequired, Core: CoreRequired, Dependencies: []string{}}
	for _, i := range r.Instances {
		s.Facts[i.ID] = ProviderFact{Target: "local", State: ProviderNewRequired, Core: CoreRequired, Dependencies: []string{storage.ID}}
	}
	fp, err := ResolveFootprint(m, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.Additional) != 2 || len(fp.Requirements) != 1 {
		t.Fatalf("genuine backing dependency omitted or fabricated intent: %+v", fp)
	}
	m.Services.Secrets, m.Services.SQL = false, true
	fp, err = ResolveFootprint(m, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range fp.Requirements {
		if requirement.InstanceID == storage.ID {
			t.Fatal("SQL capability automatically selected private backing storage")
		}
	}
}
