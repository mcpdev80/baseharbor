package development

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

type invalidPathAdapter struct{}

func (invalidPathAdapter) Descriptor() extension.Metadata {
	return extension.Metadata{SchemaVersion: extension.DescriptorVersion, ID: "development/invalid-path", Family: extension.FamilyDevelopment, Version: "0.1.0"}
}
func (invalidPathAdapter) Detect(string) (Detection, error)     { return Detection{}, nil }
func (invalidPathAdapter) Supports(capability.Requirement) bool { return true }
func (invalidPathAdapter) Plan(_ application.PortableContract, _ StackProfile, component Component) ([]Action, error) {
	return []Action{{Kind: ActionSource, Component: component.ID, Name: "invalid"}}, nil
}
func (invalidPathAdapter) Bootstrap(DevelopmentPlan, Component) ([]GeneratedFile, error) {
	return []GeneratedFile{{Path: "../escape", Content: []byte("bad")}}, nil
}
func (invalidPathAdapter) Validate(string, application.PortableContract, Component) (Validation, error) {
	return Validation{Satisfied: true}, nil
}

func TestBootstrapProjectRollbackPreservesExistingEmptyRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := application.Manifest{Version: application.CurrentVersion, Name: "demo", Environment: "dev"}
	profile := StackProfile{
		APIVersion: StackProfileAPIVersion,
		Kind:       StackProfileKind,
		Metadata:   ProfileMetadata{Name: "invalid"},
		Components: []Component{{ID: "app", Role: "application", Adapter: "development/invalid-path"}},
	}
	registry, err := NewRegistry(invalidPathAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapProject(root, manifest, profile, registry); err == nil {
		t.Fatal("expected generated path validation failure")
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("existing project root was removed: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("project root is no longer a directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rollback left generated entries: %#v", entries)
	}
}
