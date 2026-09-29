package development

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

type testAdapter struct{ id string }

func (a testAdapter) Descriptor() extension.Metadata {
	return extension.Metadata{SchemaVersion: extension.DescriptorVersion, ID: a.id, Family: extension.FamilyDevelopment, Version: "0.1.0"}
}
func (testAdapter) Detect(string) (Detection, error)     { return Detection{}, nil }
func (testAdapter) Supports(capability.Requirement) bool { return true }
func (testAdapter) Plan(contract application.PortableContract, profile StackProfile, component Component) ([]Action, error) {
	return []Action{{Kind: ActionSource, Capability: capability.ExposureHTTP, Name: "bootstrap", Value: component.Role}}, nil
}
func (testAdapter) Bootstrap(DevelopmentPlan, Component) ([]GeneratedFile, error) { return nil, nil }
func (testAdapter) Validate(string, application.PortableContract, Component) (Validation, error) {
	return Validation{Satisfied: true}, nil
}

func TestStackProfileSupportsMultipleComponents(t *testing.T) {
	profile := StackProfile{
		SchemaVersion: StackProfileVersion,
		Name:          "fullstack",
		Components: []Component{
			{ID: "frontend", Role: "frontend", Adapter: "development/nextjs"},
			{ID: "backend", Role: "backend", Adapter: "development/go"},
		},
		Capabilities: []CapabilityPreference{{
			Capability: capability.SQL,
			Components: []string{"backend"},
		}},
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestStackProfileRejectsUnknownCapabilityComponent(t *testing.T) {
	profile := StackProfile{
		SchemaVersion: StackProfileVersion,
		Name:          "bad",
		Components:    []Component{{ID: "backend", Role: "backend", Adapter: "development/go"}},
		Capabilities:  []CapabilityPreference{{Capability: capability.SQL, Components: []string{"worker"}}},
	}
	if err := profile.Validate(); err == nil {
		t.Fatal("Validate() expected unknown component error")
	}
}

func TestBuildPlanIsDeterministicAcrossComponents(t *testing.T) {
	registry, err := NewRegistry(testAdapter{id: "development/go"}, testAdapter{id: "development/nextjs"})
	if err != nil {
		t.Fatal(err)
	}
	profile := StackProfile{
		SchemaVersion: StackProfileVersion,
		Name:          "fullstack",
		Components: []Component{
			{ID: "frontend", Role: "frontend", Adapter: "development/nextjs"},
			{ID: "backend", Role: "backend", Adapter: "development/go"},
		},
	}
	plan, err := BuildPlan(application.PortableContract{Application: "catalog"}, profile, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(plan.Actions))
	}
	if plan.Actions[0].Component != "backend" || plan.Actions[1].Component != "frontend" {
		t.Fatalf("actions are not deterministic: %#v", plan.Actions)
	}
}
