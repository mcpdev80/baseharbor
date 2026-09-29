package development

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestGoAdapterBootstrapRoundTrip(t *testing.T) {
	contract := application.PortableContract{
		Application: "demo-go",
		Capabilities: []capability.Requirement{
			{Kind: capability.SQL, Name: "default"},
			{Kind: capability.KeyValue, Name: "default"},
			{Kind: capability.ObjectStorageS3, Name: "default"},
			{Kind: capability.Secrets, Name: "default"},
			{Kind: capability.ExposureHTTP, Name: "web"},
			{Kind: capability.TelemetryOTLP, Name: "default"},
		},
	}
	profile := StackProfile{
		APIVersion: StackProfileAPIVersion,
		Kind:       StackProfileKind,
		Metadata:     ProfileMetadata{Name: "go-service"},
		Components: []Component{{
			ID:      "app",
			Role:    "backend",
			Adapter: GoAdapterID,
		}},
	}
	registry, err := NewRegistry(GoAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(contract, profile, registry)
	if err != nil {
		t.Fatal(err)
	}
	files, err := (GoAdapter{}).Bootstrap(plan, profile.Components[0])
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, file.Content, mode); err != nil {
			t.Fatal(err)
		}
	}
	validation, err := (GoAdapter{}).Validate(root, contract, profile.Components[0])
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Satisfied {
		t.Fatalf("round-trip not satisfied: %#v", validation)
	}
	for _, requirement := range contract.Capabilities {
		if !validation.Capabilities[requirement.Kind] {
			t.Fatalf("capability %q was not rediscovered", requirement.Kind)
		}
	}
}
