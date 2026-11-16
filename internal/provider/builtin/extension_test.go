package builtin

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

func TestExtensionMetadataProjectsProviderIdentity(t *testing.T) {
	descriptor, err := Lookup(capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	metadata := ExtensionMetadata(descriptor)
	if err := metadata.Validate(); err != nil {
		t.Fatalf("extension metadata validation failed: %v", err)
	}
	if metadata.Family != extension.FamilyProvider {
		t.Fatalf("family = %q, want provider", metadata.Family)
	}
	if metadata.ID != descriptor.ID || metadata.Version != descriptor.Version {
		t.Fatalf("identity mismatch: %#v vs %#v", metadata, descriptor)
	}
}
