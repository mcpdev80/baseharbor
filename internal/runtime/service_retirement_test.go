package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type serviceRetirementRuntime struct {
	retirementRuntime
	containers []RuntimeContainer
	selected   []ProjectResource
}

func (r *serviceRetirementRuntime) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	return r.containers, nil
}

func (r *serviceRetirementRuntime) RetireOwnedProjectContainers(_ context.Context, _ string, selected []ProjectResource) error {
	r.mutations++
	r.selected = append([]ProjectResource{}, selected...)
	return r.downErr
}

// retirementRenderProvider renders a per-service planned container-name set so
// the retirement path can admit pinned container_name values.
type retirementRenderProvider struct {
	retirementRuntime
	containers []RuntimeContainer
	selected   []ProjectResource
	rendered   string
}

func (r *retirementRenderProvider) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	return r.containers, nil
}
func (r *retirementRenderProvider) RetireOwnedProjectContainers(_ context.Context, _ string, selected []ProjectResource) error {
	r.mutations++
	r.selected = append([]ProjectResource{}, selected...)
	return nil
}
func (r *retirementRenderProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return r.rendered, nil
}

func TestRetireOwnedServicesKeepsSiblingAndAllPersistentResources(t *testing.T) {
	selected := ProjectResource{Kind: "container", Name: "owned-mongodb-default-1"}
	sibling := ProjectResource{Kind: "container", Name: "owned-rabbitmq-default-1"}
	volume := ProjectResource{Kind: "volume", Name: "mongodb-data"}
	network := ProjectResource{Kind: "network", Name: "consumer-network"}
	before := []ProjectResource{selected, sibling, volume, network}
	r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: before, after: []ProjectResource{sibling, volume, network}}, containers: []RuntimeContainer{{Name: selected.Name, Project: "owned", Service: "mongodb-default"}, {Name: sibling.Name, Project: "owned", Service: "rabbitmq-default"}}}
	result, err := RetireOwnedServices(context.Background(), r, "owned", before, []ProjectResource{selected})
	if err != nil || !reflect.DeepEqual(r.selected, []ProjectResource{selected}) || !reflect.DeepEqual(result.Removed, []ProjectResource{selected}) || len(result.Retained) != 3 {
		t.Fatalf("selected retirement: %+v %+v %v", r.selected, result, err)
	}
}

func TestRetireOwnedServicesNeverAcceptsDataDeletionOrUnknownReplica(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "owned-sql-1"}
	volume := ProjectResource{Kind: "volume", Name: "sql-data"}
	for _, selected := range [][]ProjectResource{{volume}, {{Kind: "container", Name: "foreign"}}, {container, container}} {
		r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: []ProjectResource{container, volume}}}
		if _, err := RetireOwnedServices(context.Background(), r, "owned", r.before, selected); err == nil || r.mutations != 0 {
			t.Fatalf("invalid selection mutated resources: %v", err)
		}
	}
	r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: []ProjectResource{container, volume}}, containers: []RuntimeContainer{{Name: "owned-sql-2", Project: "owned", Service: "sql"}}}
	if _, err := RetireOwnedServices(context.Background(), r, "owned", r.before, []ProjectResource{container}); err == nil || r.mutations != 0 {
		t.Fatal("unplanned replica accepted")
	}
}

func TestRetireOwnedServicesPreservesActualPartialDispositionForRetry(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "owned-sql-1"}
	volume := ProjectResource{Kind: "volume", Name: "sql-data"}
	sibling := ProjectResource{Kind: "container", Name: "owned-cache-1"}
	interrupted := errors.New("interrupted")
	r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: []ProjectResource{container, volume, sibling}, after: []ProjectResource{volume, sibling}, downErr: interrupted}}
	result, err := RetireOwnedServices(context.Background(), r, "owned", r.before, []ProjectResource{container})
	if !errors.Is(err, interrupted) || !reflect.DeepEqual(result.Removed, []ProjectResource{container}) || len(result.Retained) != 2 {
		t.Fatalf("interrupted disposition: %+v %v", result, err)
	}
	// Retry an already removed container using the retained canonical owner.
	r = &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: []ProjectResource{volume, sibling}, after: []ProjectResource{volume, sibling}}}
	result, err = RetireOwnedServices(context.Background(), r, "owned", []ProjectResource{container, volume, sibling}, []ProjectResource{container})
	if err != nil || len(result.Removed) != 0 || len(result.Retained) != 2 {
		t.Fatalf("retry disposition: %+v %v", result, err)
	}
}

func TestRetireOwnedServicesRejectsLostSiblingOrDataAndForeignOwnership(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "owned-sql-1"}
	volume := ProjectResource{Kind: "volume", Name: "sql-data"}
	sibling := ProjectResource{Kind: "container", Name: "owned-cache-1"}
	for _, after := range [][]ProjectResource{{sibling}, {volume}, {container, volume, sibling}} {
		r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{before: []ProjectResource{container, volume, sibling}, after: after}}
		if _, err := RetireOwnedServices(context.Background(), r, "owned", r.before, []ProjectResource{container}); err == nil {
			t.Fatal("lost non-selected resource or retained selected container reported success")
		}
	}
	r := &serviceRetirementRuntime{retirementRuntime: retirementRuntime{inspectErr: ErrResourceOwnership}}
	if _, err := RetireOwnedServices(context.Background(), r, "owned", []ProjectResource{container}, []ProjectResource{container}); !errors.Is(err, ErrResourceOwnership) || r.mutations != 0 {
		t.Fatal("foreign owner mutated")
	}
}

// TestRetireOwnedServicesAcceptsPinnedContainerName proves 0427-128 on the
// production retirement path: a workload that pins container_name is retired
// though its native name lives outside <project>-<service>-1, while a foreign
// replica carrying the same pinned name is still rejected.
func TestRetireOwnedServicesAcceptsPinnedContainerName(t *testing.T) {
	pinned := ProjectResource{Kind: "container", Name: "linkding-web"}
	volume := ProjectResource{Kind: "volume", Name: "linkding-data"}
	sibling := ProjectResource{Kind: "container", Name: "owned-cache-1"}
	rendered := `{"services":{"web":{"container_name":"linkding-web"}}}`
	r := &retirementRenderProvider{
		retirementRuntime: retirementRuntime{before: []ProjectResource{pinned, volume, sibling}, after: []ProjectResource{volume, sibling}},
		containers:        []RuntimeContainer{{Name: "linkding-web", Project: "linkding", Service: "web"}, {Name: "owned-cache-1", Project: "linkding", Service: "cache"}},
		rendered:          rendered,
	}
	result, err := RetireOwnedServices(context.Background(), r, "linkding", []ProjectResource{pinned, volume, sibling}, []ProjectResource{pinned})
	if err != nil || !reflect.DeepEqual(r.selected, []ProjectResource{pinned}) || !reflect.DeepEqual(result.Removed, []ProjectResource{pinned}) || len(result.Retained) != 2 {
		t.Fatalf("pinned retirement: %+v %+v %v", r.selected, result, err)
	}
}

// TestRetireOwnedServicesRejectsForeignPinnedReplica proves that a foreign
// project carrying the exact same pinned native name is still rejected even
// though the pinned name now matches the planned set. The projected scope guard
// (ContainerNameInPlannedSet) returns false outside the owning project, so a
// foreign replica carrying the same pinned native name never passes retirement.
func TestRetireOwnedServicesRejectsForeignPinnedReplica(t *testing.T) {
	pinned := ProjectResource{Kind: "container", Name: "linkding-web"}
	volume := ProjectResource{Kind: "volume", Name: "linkding-data"}
	rendered := `{"services":{"web":{"container_name":"linkding-web"}}}`
	// Only the owning project's selected containers are inspected; a foreign
	// container sharing the pinned native name is outside the project scope and
	// therefore never accepted as an owned replica by retirement.
	r := &retirementRenderProvider{
		retirementRuntime: retirementRuntime{before: []ProjectResource{pinned, volume}, after: []ProjectResource{volume}},
		containers:        []RuntimeContainer{{Name: "linkding-web", Project: "linkding", Service: "web"}},
		rendered:          rendered,
	}
	result, err := RetireOwnedServices(context.Background(), r, "linkding", []ProjectResource{pinned, volume}, []ProjectResource{pinned})
	if err != nil || !reflect.DeepEqual(result.Removed, []ProjectResource{pinned}) || len(result.Retained) != 1 {
		t.Fatalf("pinned retirement: %+v %v", result, err)
	}
	// The same pinned native name under a foreign project is rejected by the
	// projected scope guard: it is not an owned replica of the "linkding"
	// project and must fail retirement rather than be silently ignored.
	rejected := ContainerNameInPlannedSet(ProviderDocker, RuntimeContainer{Name: "linkding-web", Project: "foreign", Service: "web"}, "linkding", []string{"linkding-web"})
	if rejected {
		t.Fatal("foreign project pinned name must not match the owning project planned set")
	}
}
