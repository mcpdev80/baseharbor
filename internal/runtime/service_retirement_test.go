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
	renderErr  error
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
	return r.rendered, r.renderErr
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

// TestRetireOwnedServicesRejectsForeignPinnedReplica proves on the production
// retirement call (not only the matcher) that a runtime inventory which also
// carries a FOREIGN-project row with the exact same pinned native name and the
// same service label never legitimizes that foreign container: the foreign row
// must not be accepted as an owned replica, must not be selected and must not
// be mutated, and the same-name foreign row must not make the own pinned
// replica fail closed either (0427-128 r5, recipe point 2/3).
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
	// The matcher contract stays: the same pinned native name under a foreign
	// project is never an owned replica of the owning project's planned set.
	rejected := ContainerNameInPlannedSet(ProviderDocker, RuntimeContainer{Name: "linkding-web", Project: "foreign", Service: "web"}, "linkding", []string{"linkding-web"})
	if rejected {
		t.Fatal("foreign project pinned name must not match the owning project planned set")
	}
	// Production path: the full runtime inventory additionally carries a
	// foreign-project row with the exact same pinned native name and service.
	// Retirement must neither accept that row as an owned replica, nor select
	// or mutate it, nor let it disturb the own pinned retirement.
	r = &retirementRenderProvider{
		retirementRuntime: retirementRuntime{before: []ProjectResource{pinned, volume}, after: []ProjectResource{volume}},
		containers: []RuntimeContainer{
			{Name: "linkding-web", Project: "linkding", Service: "web"},
			{Name: "linkding-web", Project: "foreign", Service: "web", Running: true},
		},
		rendered: rendered,
	}
	result, err = RetireOwnedServices(context.Background(), r, "linkding", []ProjectResource{pinned, volume}, []ProjectResource{pinned})
	if err != nil || !reflect.DeepEqual(result.Removed, []ProjectResource{pinned}) || len(result.Retained) != 1 {
		t.Fatalf("cross-project pinned production retirement: %+v %v", result, err)
	}
	if !reflect.DeepEqual(r.selected, []ProjectResource{pinned}) {
		t.Fatalf("cross-project pinned replica changed the mutation selection: %+v", r.selected)
	}
	// Foreign-only inventory: only the foreign same-name row is live. The
	// production call must never treat it as an owned replica: the own pinned
	// container remains (verification error for the retired target), the
	// foreign row is never selected, and nothing about it fails the check
	// open.
	foreignOnly := &retirementRenderProvider{
		retirementRuntime: retirementRuntime{before: []ProjectResource{pinned, volume}, after: []ProjectResource{pinned, volume}},
		containers:        []RuntimeContainer{{Name: "linkding-web", Project: "foreign", Service: "web", Running: true}},
		rendered:          rendered,
	}
	if _, err := RetireOwnedServices(context.Background(), foreignOnly, "linkding", []ProjectResource{pinned, volume}, []ProjectResource{pinned}); err == nil || !reflect.DeepEqual(foreignOnly.selected, []ProjectResource{pinned}) {
		t.Fatal("foreign-only pinned row changed the retirement contract", err)
	}
}

// TestRetireOwnedServicesRenderFallbackStaysFailClosed proves 0427-128 r5: when
// the optional render capability is present but yields no verifiable model
// (render error or empty model), the identity check falls back to the canonical
// ordinal name fail-closed; a pinned replica is not silently accepted on an
// unverifiable render.
func TestRetireOwnedServicesRenderFallbackStaysFailClosed(t *testing.T) {
	pinnedContainer := RuntimeContainer{Name: "linkding-web", Project: "linkding", Service: "web"}
	resource := ProjectResource{Kind: "container", Name: "linkding-web-1"}
	volume := ProjectResource{Kind: "volume", Name: "linkding-data"}
	for _, model := range []struct {
		name     string
		rendered string
		err      error
	}{
		{"render-error", ``, errors.New("docker unavailable")},
		{"empty-model", ``, nil},
		{"malformed-model", `{"services":`, nil},
	} {
		t.Run(model.name, func(t *testing.T) {
			r := &retirementRenderProvider{
				retirementRuntime: retirementRuntime{before: []ProjectResource{resource, volume}, after: []ProjectResource{volume}},
				containers:        []RuntimeContainer{pinnedContainer},
				rendered:          model.rendered,
				renderErr:         model.err,
			}
			if _, err := RetireOwnedServices(context.Background(), r, "linkding", []ProjectResource{resource, volume}, []ProjectResource{resource}); err == nil || r.mutations != 0 {
				t.Fatalf("unverifiable render (%s) admitted a pinned replica fail-open: %v", model.name, err)
			}
		})
	}
}
