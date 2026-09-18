package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRegisterReferenceProvidersMapsCurrentOwnership(t *testing.T) {
	registry := capability.NewRegistry()
	alpha := New("alpha", "production", true, true, true)
	beta := New("beta", "production", false, false, true)

	if err := registerReferenceProviders(&registry, alpha); err != nil {
		t.Fatal(err)
	}
	if err := registerReferenceProviders(&registry, beta); err != nil {
		t.Fatal(err)
	}

	if len(registry.Instances) != 3 {
		t.Fatalf("instances=%#v", registry.Instances)
	}
	shared, err := registry.Resolve(capability.ProviderOpenBao, capability.ScopeShared, "beta", "")
	if err != nil {
		t.Fatal(err)
	}
	if shared.ID != "openbao/control-plane" {
		t.Fatalf("shared=%#v", shared)
	}
	pg, err := registry.Resolve(capability.ProviderPostgreSQL, capability.ScopeApplication, "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if pg.OwnerApplication != "alpha" {
		t.Fatalf("postgres=%#v", pg)
	}
	if _, err := registry.Resolve(capability.ProviderPostgreSQL, capability.ScopeApplication, "beta", ""); err == nil {
		t.Fatal("beta unexpectedly resolved alpha dedicated PostgreSQL")
	}
}
