package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type sharedConvergenceRuntime struct {
	bhruntime.RuntimeProvider
	resources []bhruntime.ProjectResource
	running   map[string]bool
	project   string
	mutations int
	foreign   bool
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
	for _, service := range services {
		r.running[service] = true
	}
	return nil
}
func (r *sharedConvergenceRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	var result []bhruntime.RuntimeContainer
	for service, running := range r.running {
		result = append(result, bhruntime.RuntimeContainer{Project: r.project, Service: service, Running: running})
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
			}
		}
		for path, content := range material {
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != content {
				t.Fatalf("retained material changed: %s %v", path, err)
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
