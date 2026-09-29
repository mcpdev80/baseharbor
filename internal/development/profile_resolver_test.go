package development

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProfileCatalogResolveComposesDeterministically(t *testing.T) {
	catalog := ProfileCatalog{
		"development/go": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "development/go"},
			Components: []Component{{ID: "backend", Role: "backend", Adapter: "development/go"}},
		},
		"database/postgres-pgx": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "database/postgres-pgx"},
			Components: []Component{{ID: "backend", Role: "backend", Adapter: "development/go"}},
			Capabilities: []CapabilityPreference{{
				Capability:               capability.SQL,
				Components:               []string{"backend"},
				ImplementationPreference: "postgres",
				DevelopmentIntegration:   "pgx",
			}},
		},
		"company/api": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "company/api"},
			Extends:    []string{"development/go", "database/postgres-pgx"},
			Components: []Component{{ID: "backend", Role: "backend", Adapter: "development/go"}},
		},
	}
	got, err := catalog.Resolve("company/api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Name != "company/api" || len(got.Components) != 1 || len(got.Capabilities) != 1 {
		t.Fatalf("unexpected effective profile: %#v", got)
	}
}

func TestProfileCatalogResolveRejectsConflict(t *testing.T) {
	catalog := ProfileCatalog{
		"a": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "a"},
			Components: []Component{{ID: "app", Role: "backend", Adapter: "development/go"}},
		},
		"b": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "b"},
			Components: []Component{{ID: "app", Role: "frontend", Adapter: "development/nextjs"}},
		},
		"root": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "root"},
			Extends:    []string{"a", "b"},
			Components: []Component{{ID: "root", Role: "service", Adapter: "development/go"}},
		},
	}
	if _, err := catalog.Resolve("root"); err == nil {
		t.Fatal("expected composition conflict")
	}
}

func TestProfileCatalogResolveRejectsCycle(t *testing.T) {
	catalog := ProfileCatalog{
		"a": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "a"},
			Extends:    []string{"b"},
			Components: []Component{{ID: "a", Role: "backend", Adapter: "development/go"}},
		},
		"b": {
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "b"},
			Extends:    []string{"a"},
			Components: []Component{{ID: "b", Role: "backend", Adapter: "development/go"}},
		},
	}
	if _, err := catalog.Resolve("a"); err == nil {
		t.Fatal("expected composition cycle")
	}
}
