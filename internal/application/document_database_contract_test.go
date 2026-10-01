package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestDocumentDatabaseManifestContractAndPlan(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "catalog",
		Environment:   "dev",
		Services: Services{
			DocumentDatabase:          true,
			DocumentDatabaseInstances: map[string]ServiceInstance{"documents": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	rendered := m.YAML()
	for _, want := range []string{"document_database:", "documents: {}"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("manifest missing %q:\n%s", want, rendered)
		}
	}
	parsed, err := ParseYAML(rendered)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := PortableContractFromManifest(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, requirement := range contract.Capabilities {
		if requirement.Kind == capability.DocumentDatabase {
			found = true
			if requirement.Name != "documents" {
				t.Fatalf("document database name = %q", requirement.Name)
			}
		}
	}
	if !found {
		t.Fatal("portable contract missing database.document capability")
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	var provider capability.ProviderKind
	for _, resource := range resources {
		if resource.Kind == capability.DocumentDatabase {
			provider = resource.Provider
		}
	}
	if provider != capability.ProviderMongoDB {
		t.Fatalf("document database provider = %q", provider)
	}
	plan, err := BuildPlan(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var joined string
	for _, action := range plan.Actions {
		joined += action.Resource + " " + action.Description + "\n"
	}
	for _, want := range []string{"database.document:documents", "document write/read"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("plan missing %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"replica", "wiredtiger", "mongodb://"} {
		if strings.Contains(strings.ToLower(rendered), forbidden) {
			t.Fatalf("portable manifest leaked provider topology %q:\n%s", forbidden, rendered)
		}
	}
}
