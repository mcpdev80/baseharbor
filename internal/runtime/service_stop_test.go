package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func (r *selectedStopRuntime) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return r.rendered, nil
}

type selectedStopRuntime struct {
	before, after                      []ProjectResource
	containers, stopped                []RuntimeContainer
	selected                           []string
	inspectCalls, listCalls, mutations int
	inspectErr, stopErr                error
	rendered                           string
}

func (r *selectedStopRuntime) Kind() ProviderKind { return ProviderDocker }
func (r *selectedStopRuntime) InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error) {
	r.inspectCalls++
	if r.inspectCalls == 1 {
		return r.before, r.inspectErr
	}
	return r.after, nil
}
func (r *selectedStopRuntime) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	r.listCalls++
	if r.listCalls == 1 {
		return r.containers, nil
	}
	return r.stopped, nil
}
func (r *selectedStopRuntime) StopProjectFilesSelected(_ context.Context, _ string, _ string, _ map[string]string, services []string, _ ...string) error {
	r.mutations++
	r.selected = services
	return r.stopErr
}
func selectedStopFixture() *selectedStopRuntime {
	resources := []ProjectResource{{Kind: "container", Name: "owned-postgres-1"}, {Kind: "volume", Name: "sql-data"}, {Kind: "network", Name: "consumer-network"}}
	containers := []RuntimeContainer{{ID: "sql-id", Name: "owned-postgres-1", Project: "owned", Service: "postgres", Running: true, State: "running"}, {ID: "object-id", Name: "owned-seaweedfs-1", Project: "owned", Service: "seaweedfs", Running: true, State: "running"}}
	stopped := append([]RuntimeContainer{}, containers...)
	stopped[0].Running, stopped[0].State = false, "exited"
	return &selectedStopRuntime{before: resources, after: resources, containers: containers, stopped: stopped}
}
func stopFixture(ctx context.Context, r *selectedStopRuntime) (RuntimeServiceStopResult, error) {
	return StopOwnedProjectServices(ctx, r, "owned", "/protected", nil, r.before, []string{"postgres"}, "/protected/compose.yaml")
}
func TestSelectedStopPreservesDataNetworksAndRunningProjectSiblings(t *testing.T) {
	r := selectedStopFixture()
	result, err := stopFixture(t.Context(), r)
	if err != nil || !result.Verified || !reflect.DeepEqual(r.selected, []string{"postgres"}) || len(result.Stopped) != 1 || !reflect.DeepEqual(result.Retained, r.before) {
		t.Fatalf("selected stop result: %+v %v", result, err)
	}
}
func TestSelectedStopRejectsForeignOwnerReplicaAndDataSelectionBeforeMutation(t *testing.T) {
	for _, kind := range []string{"owner", "replica", "invalid-service"} {
		t.Run(kind, func(t *testing.T) {
			r := selectedStopFixture()
			switch kind {
			case "owner":
				r.inspectErr = ErrResourceOwnership
			case "replica":
				r.containers[0].Name = "owned-postgres-2"
			case "invalid-service":
				r.before[0].Kind = "volume"
			}
			if _, err := stopFixture(t.Context(), r); err == nil || r.mutations != 0 {
				t.Fatal("unsafe selection mutated", err)
			}
		})
	}
}
func TestSelectedStopReturnsActualPartialResultAndAllowsStoppedRetry(t *testing.T) {
	r := selectedStopFixture()
	r.stopErr = errors.New("interrupted")
	result, err := stopFixture(t.Context(), r)
	if !errors.Is(err, r.stopErr) || result.Verified || len(result.Stopped) != 1 || len(result.Retained) != 3 {
		t.Fatalf("partial result: %+v %v", result, err)
	}
	r = selectedStopFixture()
	r.containers = append([]RuntimeContainer{}, r.stopped...)
	result, err = stopFixture(t.Context(), r)
	if err != nil || !result.Verified {
		t.Fatalf("stopped retry: %+v %v", result, err)
	}
}

func TestSelectedStopAcceptsNativeProcessRetirementButNeverLosesSiblingOrData(t *testing.T) {
	r := selectedStopFixture()
	r.after = r.after[1:]
	r.stopped = r.stopped[1:]
	result, err := stopFixture(t.Context(), r)
	if err != nil || !result.Verified || len(result.Removed) != 1 || len(result.Stopped) != 1 || len(result.Retained) != 2 {
		t.Fatalf("native process retirement: %+v %v", result, err)
	}
	// An interrupted/repeated stop must also preserve the old canonical owner
	// when the selected process was already removed by its first native stop.
	r = selectedStopFixture()
	canonical := append([]ProjectResource{}, r.before...)
	r.before, r.after = r.before[1:], r.after[1:]
	r.containers, r.stopped = r.containers[1:], r.stopped[1:]
	result, err = StopOwnedProjectServices(t.Context(), r, "owned", "/protected", nil, canonical, []string{"postgres"}, "/protected/compose.yaml")
	if err != nil || !result.Verified || r.mutations != 0 || len(result.Retained) != 2 {
		t.Fatalf("removed-process retry: %+v %v", result, err)
	}
	r = selectedStopFixture()
	r.after, r.stopped = r.after[1:], r.stopped[1:]
	r.stopped = append(r.stopped, RuntimeContainer{ID: "unknown-replica", Project: "owned", Service: "postgres", Name: "owned-postgres-2", Running: true, State: "running"})
	result, err = stopFixture(t.Context(), r)
	if err == nil || result.Verified {
		t.Fatal("unknown new replica was hidden by original process removal")
	}
}

// TestSelectedStopAcceptsPinnedContainerName proves 0427-128 on the production
// selected-stop path: a workload that pins container_name is verified stopped
// though its native name lives outside <project>-<service>-1.
func TestSelectedStopAcceptsPinnedContainerName(t *testing.T) {
	pinnedContainer := RuntimeContainer{ID: "pinned-id", Name: "linkding-pinned-web", Project: "linkding", Service: "web", Running: true, State: "running"}
	stopped := RuntimeContainer{ID: "pinned-id", Name: "linkding-pinned-web", Project: "linkding", Service: "web", Running: false, State: "exited"}
	r := &selectedStopRuntime{
		before:     []ProjectResource{{Kind: "container", Name: "linkding-web-1"}, {Kind: "volume", Name: "linkding-data"}},
		after:      []ProjectResource{{Kind: "volume", Name: "linkding-data"}},
		containers: []RuntimeContainer{pinnedContainer},
		stopped:    []RuntimeContainer{stopped},
		rendered:   `{"services":{"web":{"container_name":"linkding-pinned-web"}}}`,
	}
	result, err := StopOwnedProjectServices(t.Context(), r, "linkding", "/protected", nil, r.before, []string{"web"}, "/protected/compose.yaml")
	if err != nil || !result.Verified || len(result.Stopped) != 1 {
		t.Fatalf("pinned selected stop: %+v %v", result, err)
	}
}

// TestSelectedStopAcceptsRetiredPinnedNativeContainer proves that backup
// may remove a selected, explicitly named Docker container while preserving
// all declared data volumes and unrelated processes. An absent selected
// process is not an absent retained ownership resource.
func TestSelectedStopAcceptsRetiredPinnedNativeContainer(t *testing.T) {
	pinned := RuntimeContainer{ID: "pinned-id", Name: "linkding-pinned-web", Project: "linkding", Service: "web", Running: true, State: "running"}
	resources := []ProjectResource{
		{Kind: "container", Name: "linkding-web-1"},
		{Kind: "container", Name: "linkding-pinned-web"},
		{Kind: "volume", Name: "linkding-data"},
	}
	r := &selectedStopRuntime{
		before: resources,
		after: []ProjectResource{{Kind: "volume", Name: "linkding-data"}},
		containers: []RuntimeContainer{pinned},
		rendered: `{"services":{"web":{"container_name":"linkding-pinned-web"}}}`,
	}
	result, err := StopOwnedProjectServices(t.Context(), r, "linkding", "/protected", nil,
		resources, []string{"web"}, "/protected/compose.yaml")
	if err != nil || !result.Verified || len(result.Removed) != 2 || len(result.Retained) != 1 {
		t.Fatalf("selected pinned native retirement: %+v %v", result, err)
	}
}

// The same native resource is never allowed to disappear if its process was
// an unrelated service, even when a selected pinned container is retired.
func TestSelectedStopRejectsLostUnselectedPinnedContainer(t *testing.T) {
	selected := RuntimeContainer{ID: "web-id", Name: "linkding-pinned-web", Project: "linkding", Service: "web", Running: true, State: "running"}
	sibling := RuntimeContainer{ID: "sibling-id", Name: "linkding-worker-1", Project: "linkding", Service: "worker", Running: true, State: "running"}
	resources := []ProjectResource{
		{Kind: "container", Name: "linkding-web-1"},
		{Kind: "container", Name: "linkding-pinned-web"},
		{Kind: "container", Name: "linkding-worker-1"},
		{Kind: "volume", Name: "linkding-data"},
	}
	r := &selectedStopRuntime{
		before: resources,
		after: []ProjectResource{
			{Kind: "container", Name: "linkding-pinned-web"},
			{Kind: "volume", Name: "linkding-data"},
		},
		containers: []RuntimeContainer{selected, sibling},
		stopped: []RuntimeContainer{{ID: "web-id", Name: "linkding-pinned-web", Project: "linkding", Service: "web", State: "exited"}},
		rendered: `{"services":{"web":{"container_name":"linkding-pinned-web"},"worker":{}}}`,
	}
	result, err := StopOwnedProjectServices(t.Context(), r, "linkding", "/protected", nil,
		resources, []string{"web"}, "/protected/compose.yaml")
	if err == nil || result.Verified {
		t.Fatalf("missing unselected container was admitted: %+v", result)
	}
}

// TestSelectedStopRejectsUnplannedPinnedReplica proves that a container of the
// selected service carrying a native name outside the rendered planned set is
// still rejected on the selected-stop path; a matching native name alone never
// infers ownership.
func TestSelectedStopRejectsUnplannedPinnedReplica(t *testing.T) {
	replicaContainer := RuntimeContainer{ID: "replica-id", Name: "linkding-other", Project: "linkding", Service: "web", Running: true, State: "running"}
	r := &selectedStopRuntime{
		before:     []ProjectResource{{Kind: "container", Name: "linkding-web-1"}, {Kind: "volume", Name: "linkding-data"}},
		after:      []ProjectResource{{Kind: "volume", Name: "linkding-data"}},
		containers: []RuntimeContainer{replicaContainer},
		stopped:    []RuntimeContainer{replicaContainer},
		rendered:   `{"services":{"web":{"container_name":"linkding-web"}}}`,
	}
	result, err := StopOwnedProjectServices(t.Context(), r, "linkding", "/protected", nil, r.before, []string{"web"}, "/protected/compose.yaml")
	if err == nil || result.Verified {
		t.Fatal("unplanned pinned replica accepted on selected stop")
	}
}
func TestSelectedStopDetectsLostDataChangedSiblingAndUnstoppedService(t *testing.T) {
	for _, kind := range []string{"volume", "sibling", "running", "identity"} {
		t.Run(kind, func(t *testing.T) {
			r := selectedStopFixture()
			switch kind {
			case "volume":
				r.after = r.after[:1]
			case "sibling":
				r.stopped[1].Running, r.stopped[1].State = false, "exited"
			case "running":
				r.stopped[0].Running, r.stopped[0].State = true, "running"
			case "identity":
				r.stopped[0].ID = "substituted"
			}
			result, err := stopFixture(t.Context(), r)
			if err == nil || result.Verified {
				t.Fatalf("false successful stop: %+v", result)
			}
		})
	}
}
