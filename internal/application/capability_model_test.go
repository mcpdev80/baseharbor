package application

import (
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestResolveCapabilityResourcesUsesReferenceProviders(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "dev",
		Services: Services{
			Postgres: true,
			Redis:    true,
			Secrets:  true,
		},
	}
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	want := []capability.Resource{
		{Application: "mailflow", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL},
		{Application: "mailflow", Kind: capability.KeyValue, Name: "default", Provider: capability.ProviderValkey},
		{Application: "mailflow", Kind: capability.Secrets, Name: "default", Provider: capability.ProviderOpenBao},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resources = %#v, want %#v", got, want)
	}
}

func TestCapabilityBindingsUseStableApplicationWorkloadIdentity(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services:    Services{Postgres: true},
	}
	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Workload != "application/mailflow" {
		t.Fatalf("bindings = %#v", bindings)
	}
	if bindings[0].Resource.Provider != capability.ProviderPostgreSQL {
		t.Fatalf("provider = %q", bindings[0].Resource.Provider)
	}
}
