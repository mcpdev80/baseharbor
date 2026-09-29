package development

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

type bootstrapAdapter struct{}

func (bootstrapAdapter) Descriptor() extension.Metadata {
	return extension.Metadata{SchemaVersion: extension.DescriptorVersion, ID: "development/test", Family: extension.FamilyDevelopment, Version: "0.1.0"}
}
func (bootstrapAdapter) Detect(string) (Detection, error) { return Detection{}, nil }
func (bootstrapAdapter) Supports(capability.Requirement) bool { return true }
func (bootstrapAdapter) Plan(_ application.PortableContract, _ StackProfile, component Component) ([]Action, error) {
	return []Action{{Kind: ActionSource, Component: component.ID, Name: "app"}}, nil
}
func (bootstrapAdapter) Bootstrap(_ DevelopmentPlan, _ Component) ([]GeneratedFile, error) {
	return []GeneratedFile{{Path: "main.txt", Content: []byte("ok\n"), Mode: 0o644}}, nil
}
func (bootstrapAdapter) Validate(string, application.PortableContract, Component) (Validation, error) {
	return Validation{Satisfied: true}, nil
}

func TestBootstrapApplicationBuildsContractPlanAndFiles(t *testing.T) {
	registry, err := NewRegistry(bootstrapAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := BootstrapApplication(NewApplicationRequest{
		Name:         "demo",
		Adapter:      "development/test",
		Capabilities: []capability.Kind{capability.ExposureHTTP, capability.SQL, capability.Secrets},
		Secrets:      []string{"APP_SECRET"},
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Name != "demo" || result.Manifest.Environment != "dev" {
		t.Fatalf("manifest = %#v", result.Manifest)
	}
	if result.Plan.Application != "demo" {
		t.Fatalf("plan application = %q", result.Plan.Application)
	}
	if len(result.FilePaths) != 2 {
		t.Fatalf("files = %#v", result.FilePaths)
	}
}

func TestWriteGeneratedFilesIsFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := WriteGeneratedFiles(root, []GeneratedFile{{Path: "../escape", Content: []byte("bad")}}); err == nil {
		t.Fatal("expected path traversal rejection")
	}
	existing := filepath.Join(root, "existing")
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteGeneratedFiles(root, []GeneratedFile{{Path: "existing", Content: []byte("replace")}}); err == nil {
		t.Fatal("expected overwrite rejection")
	}
}
