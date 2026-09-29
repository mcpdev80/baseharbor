package authoring

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestDescriptorUsesExistingProviderConformance(t *testing.T) {
	d := Descriptor{
		ID:               "example/postgresql",
		Version:          "1.0.0",
		ProviderProtocol: capability.ProviderProtocolV1,
		ServiceKinds:     []string{"sql"},
		ServiceContracts: []string{"database.sql/v1"},
		SupportedScopes:  []string{"application"},
	}
	report := Check(d)
	if report.Status != capability.ConformancePass {
		t.Fatalf("conformance = %#v", report)
	}
}

func TestInitAndLoad(t *testing.T) {
	root := t.TempDir() + "/provider"
	result, err := Init(root, "example/postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("files = %#v", result.Files)
	}
	descriptor, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.ID != "example/postgresql" {
		t.Fatalf("id = %q", descriptor.ID)
	}
}
