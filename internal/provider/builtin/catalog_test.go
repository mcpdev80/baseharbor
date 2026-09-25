package builtin

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestCatalogContainsVersionedFirstPartyProviders(t *testing.T) {
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) == 0 {
		t.Fatal("bundled provider catalog is empty")
	}
	seen := map[capability.ProviderKind]bool{}
	for _, descriptor := range catalog {
		if !strings.HasPrefix(descriptor.ID, "baseharbor/") {
			t.Fatalf("provider id %q is not namespaced", descriptor.ID)
		}
		if descriptor.Version == "" {
			t.Fatalf("provider %q has no implementation version", descriptor.ID)
		}
		if descriptor.Integration.Protocol != capability.ProviderProtocolV1 {
			t.Fatalf("provider %q protocol = %q", descriptor.ID, descriptor.Integration.Protocol)
		}
		seen[descriptor.Integration.Provider.Kind] = true
	}
	for _, required := range []capability.ProviderKind{
		capability.ProviderPostgreSQL,
		capability.ProviderValkey,
		capability.ProviderOpenBao,
		capability.ProviderSeaweedFS,
	} {
		if !seen[required] {
			t.Fatalf("provider %q missing from bundled catalog", required)
		}
	}
}

func TestLookupKeepsCapabilityAndProviderVersionsIndependent(t *testing.T) {
	descriptor, err := Lookup(capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Version != "0.1.0" {
		t.Fatalf("provider version = %q", descriptor.Version)
	}
	if len(descriptor.Integration.Capabilities) != 1 || descriptor.Integration.Capabilities[0] != capability.SQLV1.ID {
		t.Fatalf("capability versions = %#v", descriptor.Integration.Capabilities)
	}
}
