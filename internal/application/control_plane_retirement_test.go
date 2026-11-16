package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type controlPlaneRetirementRuntime struct {
	bhruntime.RuntimeProvider
	project   string
	resources []bhruntime.ProjectResource
	selected  []bhruntime.ProjectResource
	foreign   bool
}

func (r *controlPlaneRetirementRuntime) Kind() bhruntime.ProviderKind {
	return bhruntime.ProviderDocker
}
func (r *controlPlaneRetirementRuntime) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	if r.foreign {
		return nil, bhruntime.ErrResourceOwnership
	}
	return r.resources, nil
}
func (r *controlPlaneRetirementRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	containers := []bhruntime.RuntimeContainer{}
	for _, resource := range r.resources {
		if resource.Kind != "container" {
			continue
		}
		service := resource.Name[len(r.project)+1 : len(resource.Name)-2]
		containers = append(containers, bhruntime.RuntimeContainer{Project: r.project, Service: service, Name: resource.Name, Running: true})
	}
	return containers, nil
}
func (r *controlPlaneRetirementRuntime) RetireOwnedProjectContainers(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	r.selected = append(r.selected, selected...)
	remaining := []bhruntime.ProjectResource{}
	for _, resource := range r.resources {
		remove := false
		for _, target := range selected {
			if resource == target {
				remove = true
			}
		}
		if !remove {
			remaining = append(remaining, resource)
		}
	}
	r.resources = remaining
	return nil
}

func TestControlPlaneLastPermissionConsumerRetirementKeepsDataAndRecovery(t *testing.T) {
	for _, ha := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "runtime")
		files, err := bhruntime.EnsureFilesForProjectAndResources(root, bhruntime.SharedProjectName("isolated"), bhruntime.SharedResourceProjectName("isolated"), bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, ha)
		if err != nil {
			t.Fatal(err)
		}
		inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, files.Project)
		if err != nil {
			t.Fatal(err)
		}
		environment, err := os.ReadFile(files.Env)
		if err != nil {
			t.Fatal(err)
		}
		first := WithRuntimePermission(WithWorkloadComponents(New("one", "dev", false, false, false), "api"), string(capability.MetricsV1.ID), []string{"api"}, "runtime.get")
		second := WithWorkloadComponents(New("two", "dev", false, false, false), "api")
		registry := capability.NewRegistry()
		if err := registerReferenceProviders(&registry, first); err != nil {
			t.Fatal(err)
		}
		storage := RuntimeStorageDependency("isolated")
		snapshot := ResolutionSnapshot{Target: "isolated", Registry: registry, Ancillary: []capability.ProviderInstance{storage}, DependencyRoots: []string{}, Facts: map[string]ProviderFact{}}
		snapshot.Facts[storage.ID] = ProviderFact{Target: "isolated", Ancillary: true, OwnerReference: root, State: ProviderRunning, Core: CoreRequired, Dependencies: []string{}}
		for _, instance := range registry.Instances {
			snapshot.Facts[instance.ID] = ProviderFact{Target: "isolated", State: ProviderRunning, Core: CoreRequired, Dependencies: []string{storage.ID}}
		}
		original := append([]bhruntime.ProjectResource{}, inventory.Resources...)
		runtime := &controlPlaneRetirementRuntime{project: files.Project, resources: append([]bhruntime.ProjectResource{}, original...)}
		for _, remainingConsumer := range []bool{true, false} {
			first.Runtime.Permissions = nil
			if remainingConsumer {
				second.Runtime.Permissions = WithRuntimePermission(second, string(capability.MetricsV1.ID), []string{"api"}, "runtime.get").Runtime.Permissions
			} else {
				second.Runtime.Permissions = nil
			}
			consumers := []ConsumerIntent{}
			for index, m := range []Manifest{first, second} {
				roots, err := RuntimeDependencyRoots(m, registry)
				if err != nil {
					t.Fatal(err)
				}
				key := "one"
				if index == 1 {
					key = "two"
				}
				consumers = append(consumers, ConsumerIntent{Key: key, Active: true, Manifest: m, DependencyRoots: roots})
			}
			plan, err := ResolveProviderPlan(snapshot, consumers, true)
			if err != nil || !plan.Complete {
				t.Fatalf("permission-only union: %+v %v", plan, err)
			}
			result, err := RetireUnneededControlPlaneServices(context.Background(), runtime, root, "isolated", plan, registry)
			if err != nil {
				t.Fatal(err)
			}
			if remainingConsumer {
				if len(runtime.selected) > 0 || !reflect.DeepEqual(runtime.resources, original) {
					t.Fatal("shared permission consumer lost runtime")
				}
				continue
			}
			if len(result.Removed) != len(inventory.SQLServices)+len(inventory.SecretsServices) {
				t.Fatalf("last consumer not retired: %+v", result)
			}
			for _, resource := range runtime.resources {
				if resource.Kind == "container" {
					t.Fatal("unneeded runtime remains")
				}
			}
			if len(result.Retained) == 0 {
				t.Fatal("retained material inventory absent")
			}
			actual, err := os.ReadFile(files.Env)
			if err != nil || string(actual) != string(environment) {
				t.Fatal("credential/recovery owner changed")
			}
			if _, err := RetireUnneededControlPlaneServices(context.Background(), runtime, root, "isolated", plan, registry); err != nil {
				t.Fatalf("retry: %v", err)
			}
			runtime.foreign = true
			before := len(runtime.selected)
			if _, err := RetireUnneededControlPlaneServices(context.Background(), runtime, root, "isolated", plan, registry); !errors.Is(err, bhruntime.ErrResourceOwnership) || len(runtime.selected) != before {
				t.Fatal("foreign ownership was mutated")
			}
		}
	}
}
