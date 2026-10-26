package exposure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type exposureRetirementRuntime struct {
	bhruntime.RuntimeProvider
	resources []bhruntime.ProjectResource
	selected  []bhruntime.ProjectResource
	failure   error
	foreign   bool
}

func (*exposureRetirementRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }
func (r *exposureRetirementRuntime) InspectProjectResources(_ context.Context, _ string, wanted []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	if r.foreign {
		return nil, errors.New("foreign native owner")
	}
	result := []bhruntime.ProjectResource{}
	for _, item := range r.resources {
		for _, expected := range wanted {
			if item == expected {
				result = append(result, item)
			}
		}
	}
	return result, nil
}
func (*exposureRetirementRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return nil, nil
}
func (r *exposureRetirementRuntime) RetireOwnedProjectContainers(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	r.selected = append(r.selected, selected...)
	kept := []bhruntime.ProjectResource{}
	for _, item := range r.resources {
		remove := false
		for _, target := range selected {
			remove = remove || item == target
		}
		if !remove {
			kept = append(kept, item)
		}
	}
	r.resources = kept
	return r.failure
}

func exposureInventoryFixture(t *testing.T) (application.Manifest, application.RuntimeFiles, RuntimeInventory) {
	t.Helper()
	m := application.WithWorkloadComponents(application.New("owned", "dev", false, false, false), "api")
	runtime := application.RuntimeFiles{Dir: t.TempDir(), Namespace: "isolated", ResourceProject: "owned-runtime"}
	if err := os.Chmod(runtime.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	files := FilesFor(runtime)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := State{Version: stateVersion, Project: ProjectNameForRuntime(m, runtime), Network: application.ApplicationExposureNetworkNameForProject(runtime.ResourceProject), Host: "owned.baha.localhost", Routes: []Route{{Name: "public", Service: "api", TargetPort: 8080, Protocol: "https", ProviderProtocol: "http", WorkloadProtocol: "http", Visibility: "public", PublishedPort: 18080}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{files.State: data, files.Compose: []byte(composeYAML(state, files)), files.Env: []byte("# BaseHarbor managed Caddy exposure provider\n")} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := ExistingRuntimeInventory(m, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return m, runtime, inventory
}

func TestExposureRetirementPreservesSiblingAndOwnerForRetry(t *testing.T) {
	m, files, inventory := exposureInventoryFixture(t)
	sibling := bhruntime.ProjectResource{Kind: "container", Name: inventory.State.Project + "-api-1"}
	native := &exposureRetirementRuntime{resources: append(append([]bhruntime.ProjectResource{}, inventory.Resources...), sibling), failure: errors.New("native operation interrupted after removal")}
	before, err := os.ReadFile(inventory.Files.State)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RetireRuntime(t.Context(), native, m, files)
	if err == nil || len(result.Removed) != 1 || !reflect.DeepEqual(native.resources, []bhruntime.ProjectResource{sibling}) {
		t.Fatalf("partial disposition lost sibling/error: %+v %v", result, err)
	}
	if len(result.Retained) != 1 || result.Retained[0].Resource.Name != inventory.Files.Dir {
		t.Fatal("retained owner material was not inventoried")
	}
	native.failure = nil
	if _, err := RetireRuntime(t.Context(), native, m, files); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(inventory.Files.State)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("retry replaced route ownership")
	}
}

func TestExposureInventoryRejectsUnprovenStateBeforeNativeMutation(t *testing.T) {
	for _, damage := range []string{"foreign-project", "redirected-state", "unprotected-state", "foreign-compose", "foreign-env", "redirected-parent", "foreign-native"} {
		t.Run(damage, func(t *testing.T) {
			m, runtime, inventory := exposureInventoryFixture(t)
			native := &exposureRetirementRuntime{resources: inventory.Resources}
			switch damage {
			case "foreign-project":
				state := inventory.State
				state.Project = "foreign"
				data, _ := json.Marshal(state)
				if err := os.WriteFile(inventory.Files.State, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "redirected-state":
				foreign := filepath.Join(t.TempDir(), "state.json")
				if err := os.Rename(inventory.Files.State, foreign); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, inventory.Files.State); err != nil {
					t.Fatal(err)
				}
			case "unprotected-state":
				if err := os.Chmod(inventory.Files.State, 0644); err != nil {
					t.Fatal(err)
				}
			case "foreign-compose":
				if err := os.WriteFile(inventory.Files.Compose, []byte("services:\n  foreign: {}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign-env":
				if err := os.WriteFile(inventory.Files.Env, []byte("FOREIGN=value\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "redirected-parent":
				foreign := filepath.Join(t.TempDir(), "caddy")
				if err := os.Rename(inventory.Files.Dir, foreign); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, inventory.Files.Dir); err != nil {
					t.Fatal(err)
				}
			case "foreign-native":
				native.foreign = true
			}
			if _, err := RetireRuntime(t.Context(), native, m, runtime); err == nil || len(native.selected) != 0 {
				t.Fatal("unproven owner permitted runtime retirement")
			}
		})
	}
}
