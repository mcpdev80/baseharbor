package development_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
	"github.com/mcpdev80/baseharbor/internal/development/nextjsadapter"
)

func TestMultiComponentProfileBootstrapsAndHonorsCapabilityPlacement(t *testing.T) {
	registry, err := development.NewRegistry(goadapter.Adapter{}, nextjsadapter.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	profile := development.StackProfile{
		APIVersion: development.StackProfileAPIVersion,
		Kind:       development.StackProfileKind,
		Metadata:   development.ProfileMetadata{Name: "web-api"},
		Components: []development.Component{
			{ID: "web", Role: "frontend", Adapter: nextjsadapter.AdapterID},
			{ID: "api", Role: "backend", Adapter: goadapter.AdapterID},
		},
		Capabilities: []development.CapabilityPreference{
			{Capability: capability.ExposureHTTP, Components: []string{"web"}},
			{Capability: capability.SQL, Components: []string{"api"}},
		},
	}
	root := filepath.Join(t.TempDir(), "web-api")
	result, err := development.CreateApplication(root, development.NewApplicationRequest{
		Name:         "web-api",
		Environment:  "dev",
		Profile:      &profile,
		Capabilities: []capability.Kind{capability.ExposureHTTP, capability.SQL},
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Validation.Satisfied {
		t.Fatalf("validation not satisfied: %#v", result.Validation)
	}
	for _, path := range []string{
		"compose.yaml",
		"web/package.json",
		"api/go.mod",
		".baseharbor/stack-profile.yaml",
		".baseharbor/development-plan.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	planData, err := os.ReadFile(filepath.Join(root, ".baseharbor", "development-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	plan := string(planData)
	if !strings.Contains(plan, "github.com/jackc/pgx/v5") {
		t.Fatal("SQL dependency missing from development plan")
	}
	if strings.Count(plan, "github.com/jackc/pgx/v5") != 1 {
		t.Fatalf("SQL dependency leaked to more than one component:\n%s", plan)
	}
}

func TestDerivedProfileCanPlaceCapabilityOnInheritedComponent(t *testing.T) {
	catalog := development.ProfileCatalog{
		"go": development.BuiltinProfile(goadapter.AdapterID, "go"),
		"team-api": {
			APIVersion: development.StackProfileAPIVersion,
			Kind:       development.StackProfileKind,
			Metadata:   development.ProfileMetadata{Name: "team-api"},
			Extends:    []string{"go"},
			Capabilities: []development.CapabilityPreference{
				{Capability: capability.SQL, Components: []string{"app"}},
			},
		},
	}
	resolved, err := development.ResolveStackProfile("team-api", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Profile.Components) != 1 || resolved.Profile.Components[0].ID != "app" {
		t.Fatalf("unexpected inherited components: %#v", resolved.Profile.Components)
	}
	if len(resolved.Profile.Capabilities) != 1 || resolved.Profile.Capabilities[0].Components[0] != "app" {
		t.Fatalf("unexpected inherited placement: %#v", resolved.Profile.Capabilities)
	}
}
