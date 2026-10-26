package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func retainedNativeBackendFixture(t *testing.T) resolvedApplication {
	t.Helper()
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithDocumentDatabaseInstances(m, "first", "second")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m); err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(resolved.Store, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := application.RuntimeComposeYAMLForProject(m, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("preserved=protected\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return resolved
}

type nativeBackendRetirementFixture struct {
	bhruntime.RuntimeProvider
	before, after []bhruntime.ProjectResource
	selected      []bhruntime.ProjectResource
	inspections   int
}

func (f *nativeBackendRetirementFixture) Kind() bhruntime.ProviderKind {
	return bhruntime.ProviderDocker
}
func (f *nativeBackendRetirementFixture) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	f.inspections++
	if f.inspections == 1 {
		return f.before, nil
	}
	return f.after, nil
}
func (f *nativeBackendRetirementFixture) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return nil, nil
}
func (f *nativeBackendRetirementFixture) RetireOwnedProjectContainers(_ context.Context, _ string, resources []bhruntime.ProjectResource) error {
	f.selected = append([]bhruntime.ProjectResource{}, resources...)
	return nil
}

func TestNamedBackendRemovalRequiresImpactAndPreservesPeerData(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	prior := resolved.Manifest
	resolved.Manifest.Services.DocumentDatabaseInstances = map[string]application.ServiceInstance{"second": prior.Services.DocumentDatabaseInstances["second"]}
	requests, err := nativeBackendChangeRequests(resolved, nil)
	if err != nil || len(requests) != 1 || requests[0].Operation != "update" {
		t.Fatalf("named removal missing typed impact: %#v %v", requests, err)
	}
	files := application.RuntimeFilesFor(resolved.Store, prior)
	previous, err := application.NativeBackendResources(prior, files)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := application.NativeBackendResources(resolved.Manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	selected := releasedNativeBackendContainers(previous, desired)
	if len(selected) == 0 {
		t.Fatal("named removal omitted")
	}
	remaining := []bhruntime.ProjectResource{}
	for _, resource := range previous {
		remove := false
		for _, retired := range selected {
			remove = remove || retired == resource
		}
		if !remove {
			remaining = append(remaining, resource)
		}
	}
	f := &nativeBackendRetirementFixture{before: previous, after: remaining}
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true}
	var out bytes.Buffer
	if err := retireApplicationNativeBackendServices(context.Background(), resolved, f, plan, &out); err != nil {
		t.Fatal(err)
	}
	if len(f.selected) != len(selected) {
		t.Fatal("native removal selection changed")
	}
	for _, resource := range f.selected {
		if resource.Kind != "container" {
			t.Fatal("data deletion reached native retirement")
		}
	}
	// An interrupted prior apply retains the original manifest even after the
	// generated definition is updated. Repeating the cleanup remains possible.
	if _, err := recordDeploymentBeforeMutation(context.Background(), resolved, "applying"); err != nil {
		t.Fatal(err)
	}
	canonical, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	f = &nativeBackendRetirementFixture{before: remaining, after: remaining}
	if err := retireApplicationNativeBackendServices(context.Background(), resolved, f, plan, &out); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
}

func TestManagementUIRemovalIsIncludedWithoutDroppingProviderBinding(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	resolved.Manifest.Services.DocumentDatabaseManagementUI = true
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	resolved.Manifest.Services.DocumentDatabaseManagementUI = false
	requests, err := nativeBackendChangeRequests(resolved, nil)
	if err != nil || len(requests) != 1 || requests[0].Operation != "update" {
		t.Fatalf("management access removal bypasses acknowledgement: %#v %v", requests, err)
	}
	r, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range r.Bindings {
		if binding.Resource.Provider != capability.ProviderMongoDB {
			t.Fatal("provider selection changed")
		}
	}
}
