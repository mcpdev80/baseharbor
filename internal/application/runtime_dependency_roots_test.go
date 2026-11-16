package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRuntimePermissionDependencyDoesNotManufactureSecretsIntent(t *testing.T) {
	m := New("worker", "dev", false, false, false)
	m = WithWorkloadComponents(m, "api")
	m = WithRuntimePermission(m, string(capability.MetricsV1.ID), []string{"api"}, "runtime.get")
	registry := capability.NewRegistry()
	if err := registerReferenceProviders(&registry, m); err != nil {
		t.Fatal(err)
	}
	roots, err := RuntimeDependencyRoots(m, registry)
	if err != nil || len(roots) != 1 || roots[0] != "openbao/control-plane" {
		t.Fatalf("broker roots: %v %v", roots, err)
	}
	for _, binding := range registry.Bindings {
		if binding.Resource.Kind == capability.Secrets {
			t.Fatal("runtime dependency manufactured a portable secrets binding")
		}
	}
	if m.Services.Secrets {
		t.Fatal("runtime dependency changed application intent")
	}
	if _, err := RuntimeDependencyRoots(m, capability.NewRegistry()); err == nil {
		t.Fatal("unregistered realization was invented")
	}
}

func TestRuntimeDependencyRootsDistinguishesNoneFromUnknown(t *testing.T) {
	m := New("worker", "dev", false, false, false)
	roots, err := RuntimeDependencyRoots(m, capability.NewRegistry())
	if err != nil || roots == nil || len(roots) != 0 {
		t.Fatalf("known absence: %v %v", roots, err)
	}
	m.Consumes = []ConsumptionRequirement{{Name: "api", Component: "api", Interface: "http"}}
	roots, err = RuntimeDependencyRoots(m, capability.NewRegistry())
	if err != nil || roots != nil {
		t.Fatalf("unresolved consumption: %v %v", roots, err)
	}
}
