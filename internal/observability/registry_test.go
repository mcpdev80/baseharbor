package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestListMetricsRespectsSharingBoundaryAndApplicationAuthorization(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	sources := []MetricsSource{
		{ID: "shared-default", Provider: capability.ProviderTempo, Class: SourcePlatformProvider, Scope: capability.ScopeShared, Network: "trace", Target: "tempo:3200", Path: "/metrics"},
		{ID: "shared-team-a", Provider: capability.ProviderLoki, Class: SourcePlatformProvider, Scope: capability.ScopeShared, SharingBoundary: "team-a", Network: "logs", Target: "loki:3100", Path: "/metrics"},
		{ID: "app-a", Provider: capability.ProviderLoki, Class: SourceApplicationProvider, Scope: capability.ScopeApplication, OwnerApplication: "app-a", Network: "app-a-logs", Target: "loki:3100", Path: "/metrics"},
		{ID: "app-b", Provider: capability.ProviderLoki, Class: SourceApplicationProvider, Scope: capability.ScopeApplication, OwnerApplication: "app-b", Network: "app-b-logs", Target: "loki:3100", Path: "/metrics"},
	}
	for _, source := range sources {
		if err := Update(source); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ListMetrics(capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}, []string{"app-a"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "app-a" || got[1].ID != "shared-default" {
		t.Fatalf("default shared sources = %#v", got)
	}

	got, err = ListMetrics(capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "team-a", Ownership: capability.OwnershipBaseHarbor}, []string{"app-b"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "app-b" || got[1].ID != "shared-team-a" {
		t.Fatalf("team-a shared sources = %#v", got)
	}
}

func TestMetricsSourceRequiresExplicitReachability(t *testing.T) {
	source := MetricsSource{ID: "bad", Provider: capability.ProviderTempo, Class: SourcePlatformProvider, Scope: capability.ScopeShared, Target: "tempo:3200", Path: "/metrics"}
	if err := source.Validate(); err == nil {
		t.Fatal("metrics source without an explicit provider network was accepted")
	}
}
