package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type standaloneRetirementFixture struct{ nativeBackendRetirementFixture }

func (f *standaloneRetirementFixture) LogCollectionMode() bhruntime.LogCollectionMode {
	return bhruntime.LogCollectionSyslog
}

func TestStandaloneRetirementPreservesMaterialAndUnselectedModule(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	resolved.Manifest = application.WithObjectStorageBuckets(resolved.Manifest, "assets")
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var instance capability.ProviderInstance
	for _, candidate := range registry.Instances {
		if candidate.Provider.Kind == capability.ProviderSeaweedFS {
			instance = candidate
		}
	}
	files, err := objectstorage.EnsureProviderFilesAt(t.Context(), serviceissuer.New(t), resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, resources, err := objectstorage.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(files.Env)
	remaining := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind != "container" {
			remaining = append(remaining, resource)
		}
	}
	f := &standaloneRetirementFixture{nativeBackendRetirementFixture{before: resources, after: remaining}}
	var output bytes.Buffer
	if err := retireStandaloneApplicationProvider(t.Context(), resolved, f, instance, registry, &output); err != nil {
		t.Fatal(err)
	}
	if len(f.selected) == 0 {
		t.Fatal("retirement did not select owned module containers")
	}
	for _, resource := range f.selected {
		if resource.Kind != "container" || !strings.Contains(resource.Name, "seaweedfs") {
			t.Fatalf("unselected sibling or material retired: %+v", resource)
		}
	}
	after, _ := os.ReadFile(files.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("retirement regenerated credentials")
	}
	if !strings.Contains(output.String(), files.Dir) {
		t.Fatal("retained final location missing")
	}
	instance.Ownership = capability.OwnershipExternal
	f.selected = nil
	if err := retireStandaloneApplicationProvider(context.Background(), resolved, f, instance, registry, io.Discard); err == nil || len(f.selected) != 0 {
		t.Fatal("foreign owner mutated")
	}
}
