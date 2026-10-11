package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type sharedConvergenceRuntime struct {
	bhruntime.RuntimeProvider
	resources   []bhruntime.ProjectResource
	running     map[string]bool
	project     string
	mutations   int
	foreign     bool
	definitions map[string]bool
}

func (r *sharedConvergenceRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }

func (r *sharedConvergenceRuntime) RetireOwnedProjectContainers(_ context.Context, project string, selected []bhruntime.ProjectResource) error {
	r.mutations++
	removed := map[bhruntime.ProjectResource]bool{}
	for _, resource := range selected {
		removed[resource] = true
		service := strings.TrimSuffix(strings.TrimPrefix(resource.Name, project+"-"), "-1")
		delete(r.running, service)
		delete(r.definitions, service)
	}
	retained := []bhruntime.ProjectResource{}
	for _, resource := range r.resources {
		if !removed[resource] {
			retained = append(retained, resource)
		}
	}
	r.resources = retained
	return nil
}

func (r *sharedConvergenceRuntime) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	if r.foreign {
		return nil, bhruntime.ErrResourceOwnership
	}
	return r.resources, nil
}
func (r *sharedConvergenceRuntime) StopProjectFilesSelected(_ context.Context, _, _ string, _ map[string]string, services []string, _ ...string) error {
	r.mutations++
	for _, service := range services {
		r.running[service] = false
	}
	return nil
}
func (r *sharedConvergenceRuntime) UpProjectFilesSelected(_ context.Context, _, _ string, _ map[string]string, services []string, _ ...string) error {
	if len(services) == 0 {
		return errors.New("empty selection would start all retained providers")
	}
	r.mutations++
	if r.definitions == nil {
		r.definitions = map[string]bool{}
	}
	for _, service := range services {
		r.definitions[service] = true
		r.running[service] = true
		resource := bhruntime.ProjectResource{Kind: "container", Name: r.project + "-" + service + "-1"}
		if !ResourceNamedExists(r.resources, resource) {
			r.resources = append(r.resources, resource)
		}
	}
	return nil
}
func (r *sharedConvergenceRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	var result []bhruntime.RuntimeContainer
	for service, running := range r.running {
		result = append(result, bhruntime.RuntimeContainer{Name: r.project + "-" + service + "-1", Project: r.project, Service: service, Running: running})
	}
	return result, nil
}

func TestSharedBackendResolvedConsumerConvergencePreservesDataAndCredentials(t *testing.T) {
	data := t.TempDir()
	files := SharedBackendFilesAt(data, "isolated", "core")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", PostgresMembers: 1, PostgresAdminCredential: "credentials/admin", ValkeyMembers: 1, ValkeyAdminCredential: "credentials/cache", Applications: map[string]sharedBackendAppState{}}
	if err := writeSharedBackendState(files.State, state); err != nil {
		t.Fatal(err)
	}
	material := map[string]string{files.Compose: sharedBackendCompose(files, state), files.Env: "SHARED_POSTGRES_ADMIN_PASSWORD=test-only\n", filepath.Join(files.Dir, "retained-credentials"): "unchanged-test-only-credential"}
	for path, content := range material {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := ExistingSharedBackendInventoryAt(data, "isolated", "core")
	if err != nil {
		t.Fatal(err)
	}
	runtime := &sharedConvergenceRuntime{resources: inventory.Resources, project: files.Project, running: map[string]bool{}}
	first, second := New("one", "dev", true, true, false), New("two", "dev", true, false, false)
	registry := capability.NewRegistry()
	if err := registerReferenceProviders(&registry, first); err != nil {
		t.Fatal(err)
	}
	if err := registerReferenceProviders(&registry, second); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct{ sqlOne, sqlTwo, cache bool }{{true, true, true}, {false, true, true}, {false, false, true}, {false, false, false}, {true, false, false}} {
		first.Services.SQL, second.Services.SQL, first.Services.Cache = phase.sqlOne, phase.sqlTwo, phase.cache
		snapshot := ResolutionSnapshot{Target: "isolated", Registry: registry, DependencyRoots: []string{}, Facts: map[string]ProviderFact{}}
		for _, instance := range registry.Instances {
			snapshot.Facts[instance.ID] = ProviderFact{Target: "isolated", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}
		}
		plan, err := ResolveProviderPlan(snapshot, []ConsumerIntent{{Key: "one", Active: true, Manifest: first, DependencyRoots: []string{}}, {Key: "two", Active: true, Manifest: second, DependencyRoots: []string{}}}, true)
		if err != nil {
			t.Fatal(err)
		}
		result, err := ConvergeExistingSharedBackendRuntime(context.Background(), runtime, data, "isolated", plan, registry)
		if err != nil || len(result.Retained) == 0 {
			t.Fatalf("phase %+v: %+v %v", phase, result, err)
		}
		for kind, services := range inventory.Services {
			for _, service := range services {
				wanted := phase.cache
				if kind == capability.ProviderPostgreSQL {
					wanted = phase.sqlOne || phase.sqlTwo
				}
				if runtime.running[service] != wanted {
					t.Fatalf("phase %+v: shared service %s running=%v", phase, service, runtime.running[service])
				}
				if runtime.definitions[service] != wanted {
					t.Fatalf("phase %+v: shared service %s boot definition retained or recreated after retirement", phase, service)
				}
			}
		}
		for path, content := range material {
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != content {
				t.Fatalf("retained material changed: %s %v", path, err)
			}
		}
	}
	// Automatic convergence must honor the common plan's preserve decision,
	// including unknown runtime facts and unaccounted consumer inventory.
	for _, incomplete := range []bool{false, true} {
		snapshot := ResolutionSnapshot{Target: "isolated", Registry: registry, DependencyRoots: []string{}, Facts: map[string]ProviderFact{}}
		for _, instance := range registry.Instances {
			state := ProviderRunning
			if instance.Provider.Kind == capability.ProviderValkey && !incomplete {
				state = ProviderUnverifiable
			}
			snapshot.Facts[instance.ID] = ProviderFact{Target: "isolated", State: state, Core: CoreNotRequired, Dependencies: []string{}}
		}
		plan, err := ResolveProviderPlan(snapshot, []ConsumerIntent{{Key: "one", Active: true, Manifest: first, DependencyRoots: []string{}}, {Key: "two", Active: true, Manifest: second, DependencyRoots: []string{}}}, !incomplete)
		if err != nil {
			t.Fatal(err)
		}
		for _, service := range inventory.Services[capability.ProviderValkey] {
			runtime.running[service] = true
			resource := bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-" + service + "-1"}
			if !ResourceNamedExists(runtime.resources, resource) {
				runtime.resources = append(runtime.resources, resource)
			}
		}
		if _, err := ConvergeExistingSharedBackendRuntime(context.Background(), runtime, data, "isolated", plan, registry); err != nil {
			t.Fatal(err)
		}
		for _, service := range inventory.Services[capability.ProviderValkey] {
			if !runtime.running[service] {
				t.Fatalf("incomplete=%t automatic convergence retired a preserved provider", incomplete)
			}
		}
	}
	runtime.foreign = true
	before := runtime.mutations
	_, err = ConvergeExistingSharedBackendRuntime(context.Background(), runtime, data, "isolated", ProviderPlan{Version: ProviderPlanVersion, Target: "isolated"}, registry)
	if !errors.Is(err, bhruntime.ErrResourceOwnership) || runtime.mutations != before {
		t.Fatal("foreign ownership permitted shared service mutation")
	}
}
