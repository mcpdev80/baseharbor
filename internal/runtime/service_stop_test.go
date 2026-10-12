package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type selectedStopRuntime struct {
	before, after                      []ProjectResource
	containers, stopped                []RuntimeContainer
	selected                           []string
	inspectCalls, listCalls, mutations int
	inspectErr, stopErr                error
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
