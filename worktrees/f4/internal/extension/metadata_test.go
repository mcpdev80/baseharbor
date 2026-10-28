package extension

import "testing"

func TestMetadataValidate(t *testing.T) {
	m := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/provider",
		Family:        FamilyProvider,
		Version:       "1.2.3",
		Compatibility: Compatibility{
			BaseHarbor: ">=0.4.18 <0.6.0",
			Contracts:  []string{"database.sql/v1"},
			Platforms:  []Platform{{OS: "linux", Arch: "amd64"}},
		},
		Artifact: Artifact{
			OCIReference: "ghcr.io/example/provider:1.2.3",
			Digest:       "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			MediaType:    "application/vnd.baseharbor.extension.v1+json",
		},
		ConfigurationSchema: SchemaReference{
			Dialect: JSONSchema202012,
			URI:     "https://example.invalid/provider.schema.json",
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMetadataRejectsSemanticLeakageThroughFamily(t *testing.T) {
	m := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/thing",
		Family:        "runtime-provider",
		Version:       "1.0.0",
	}
	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected unsupported family error")
	}
}

func TestMetadataRejectsMutableOrMalformedDigest(t *testing.T) {
	m := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/provider",
		Family:        FamilyProvider,
		Version:       "1.0.0",
		Artifact: Artifact{
			OCIReference: "ghcr.io/example/provider:latest",
			Digest:       "latest",
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected digest error")
	}
}

func TestMetadataRejectsDuplicateCompatibilityEntries(t *testing.T) {
	m := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/provider",
		Family:        FamilyProvider,
		Version:       "1.0.0",
		Compatibility: Compatibility{
			Contracts: []string{"database.sql/v1", "database.sql/v1"},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected duplicate contract error")
	}
}
