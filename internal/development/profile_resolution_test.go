package development

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestResolveStackProfileChildOverridesParent(t *testing.T) {
	catalog := ProfileCatalog{
		"team/base": {
			APIVersion: StackProfileAPIVersion,
			Kind: StackProfileKind,
			Metadata: ProfileMetadata{Name: "team/base"},
			Components: []Component{{ID: "app", Role: "backend", Adapter: "development/go"}},
			Capabilities: []CapabilityPreference{{
				Capability: capability.SQL,
				Components: []string{"app"},
				DevelopmentIntegration: "pgx",
			}},
		},
		"team/custom": {
			APIVersion: StackProfileAPIVersion,
			Kind: StackProfileKind,
			Metadata: ProfileMetadata{Name: "team/custom"},
			Extends: []string{"team/base"},
			Components: []Component{{ID: "app", Role: "backend", Adapter: "development/quarkus"}},
			Capabilities: []CapabilityPreference{{
				Capability: capability.SQL,
				Components: []string{"app"},
				DevelopmentIntegration: "quarkus-jdbc-postgresql",
			}},
		},
	}
	result, err := ResolveStackProfile("team/custom", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Profile.Components[0].Adapter; got != "development/quarkus" {
		t.Fatalf("adapter = %q", got)
	}
	if got := result.Profile.Capabilities[0].DevelopmentIntegration; got != "quarkus-jdbc-postgresql" {
		t.Fatalf("integration = %q", got)
	}
}

func TestResolveStackProfileRejectsParentConflict(t *testing.T) {
	catalog := ProfileCatalog{
		"a": {
			APIVersion: StackProfileAPIVersion, Kind: StackProfileKind,
			Metadata: ProfileMetadata{Name: "a"},
			Components: []Component{{ID: "app", Role: "backend", Adapter: "development/go"}},
		},
		"b": {
			APIVersion: StackProfileAPIVersion, Kind: StackProfileKind,
			Metadata: ProfileMetadata{Name: "b"},
			Components: []Component{{ID: "app", Role: "backend", Adapter: "development/python"}},
		},
		"c": {
			APIVersion: StackProfileAPIVersion, Kind: StackProfileKind,
			Metadata: ProfileMetadata{Name: "c"},
			Extends: []string{"a", "b"},
		},
	}
	if _, err := ResolveStackProfile("c", catalog); err == nil {
		t.Fatal("expected deterministic parent conflict")
	}
}

func TestResolveStackProfileRejectsCycle(t *testing.T) {
	catalog := ProfileCatalog{
		"a": {APIVersion: StackProfileAPIVersion, Kind: StackProfileKind, Metadata: ProfileMetadata{Name: "a"}, Extends: []string{"b"}},
		"b": {APIVersion: StackProfileAPIVersion, Kind: StackProfileKind, Metadata: ProfileMetadata{Name: "b"}, Extends: []string{"a"}},
	}
	if _, err := ResolveStackProfile("a", catalog); err == nil {
		t.Fatal("expected composition cycle error")
	}
}
