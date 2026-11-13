package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestMissingExposureProjectionRequiresProtectedLocalDirectories(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "runtime")
	providers := filepath.Join(runtime, "providers")
	caddy := filepath.Join(providers, "caddy")
	if err := os.Mkdir(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if missing, err := missingExposureProjection(runtime, caddy); err != nil || !missing {
		t.Fatalf("missing providers directory was not recognized: missing=%v err=%v", missing, err)
	}
	if err := os.Mkdir(providers, 0700); err != nil {
		t.Fatal(err)
	}
	if missing, err := missingExposureProjection(runtime, caddy); err != nil || !missing {
		t.Fatalf("missing Caddy projection was not recognized: missing=%v err=%v", missing, err)
	}
	if err := os.Symlink(filepath.Join(root, "foreign"), caddy); err != nil {
		t.Fatal(err)
	}
	if _, err := missingExposureProjection(runtime, caddy); err == nil {
		t.Fatal("redirected Caddy projection was treated as absent")
	}
}

type orphanExposureRuntime struct {
	resources  []bhruntime.ProjectResource
	containers []bhruntime.RuntimeContainer
	removed    []bhruntime.ProjectResource
}

func (r *orphanExposureRuntime) InspectProjectResources(_ context.Context, _ string, _ []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return append([]bhruntime.ProjectResource{}, r.resources...), nil
}

func (r *orphanExposureRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return append([]bhruntime.RuntimeContainer{}, r.containers...), nil
}

func (r *orphanExposureRuntime) RetireOwnedProjectContainers(_ context.Context, project string, resources []bhruntime.ProjectResource) error {
	r.removed = append([]bhruntime.ProjectResource{}, resources...)
	wanted := map[bhruntime.ProjectResource]bool{}
	for _, resource := range resources {
		wanted[resource] = true
	}
	kept := r.resources[:0]
	for _, resource := range r.resources {
		if !wanted[resource] {
			kept = append(kept, resource)
		}
	}
	r.resources = kept
	for i := range r.containers {
		if r.containers[i].Project == project {
			r.containers[i].Running = false
		}
	}
	return nil
}

func TestRetireOrphanedExposureProjectWhenOnlyOwnedCaddyContainersRemain(t *testing.T) {
	project := "bh-demo-dev"
	container := bhruntime.ProjectResource{Kind: "container", Name: project + "-baseharbor-internal-exposure-public-1"}
	runtime := &orphanExposureRuntime{
		resources:  []bhruntime.ProjectResource{container},
		containers: []bhruntime.RuntimeContainer{{Name: container.Name, Project: project, Service: "baseharbor-internal-exposure-public", Running: true}},
	}
	removed, err := retireOrphanedExposureResources(context.Background(), runtime, project, runtime.resources)
	if err != nil || !reflect.DeepEqual(removed, []bhruntime.ProjectResource{container}) || len(runtime.resources) != 0 || !reflect.DeepEqual(runtime.removed, []bhruntime.ProjectResource{container}) {
		t.Fatalf("orphaned owned Caddy container was not safely retired: removed=%#v remaining=%#v selected=%#v err=%v", removed, runtime.resources, runtime.removed, err)
	}
}

func TestRetireOrphanedExposureProjectAcceptsVerifiedAbsence(t *testing.T) {
	removed, err := retireOrphanedExposureResources(context.Background(), nil, "bh-demo-dev", nil)
	if err != nil || len(removed) != 0 {
		t.Fatalf("empty per-app exposure projection was not accepted: %#v %v", removed, err)
	}
}

func TestRetireOrphanedExposureProjectKeepsUnrelatedResourcesAndRemovesOnlyCanonicalCaddy(t *testing.T) {
	project := "bh-demo-dev"
	caddy := bhruntime.ProjectResource{Kind: "container", Name: project + "-baseharbor-internal-exposure-public-1"}
	network := bhruntime.ProjectResource{Kind: "network", Name: project + "-observability"}
	volume := bhruntime.ProjectResource{Kind: "volume", Name: project + "-data"}
	runtime := &orphanExposureRuntime{resources: []bhruntime.ProjectResource{caddy, network, volume}}
	removed, err := retireOrphanedExposureResources(context.Background(), runtime, project, runtime.resources)
	if err != nil || !reflect.DeepEqual(removed, []bhruntime.ProjectResource{caddy}) {
		t.Fatalf("orphaned Caddy retirement failed or selected unrelated resources: removed=%#v err=%v", removed, err)
	}
	if !reflect.DeepEqual(runtime.removed, []bhruntime.ProjectResource{caddy}) || !reflect.DeepEqual(runtime.resources, []bhruntime.ProjectResource{network, volume}) {
		t.Fatalf("unrelated project resources were not preserved: removed=%#v remaining=%#v", runtime.removed, runtime.resources)
	}
}
