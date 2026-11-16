package runtimebroker

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type brokerRetirementRuntime struct {
	bhruntime.RuntimeProvider
	project     string
	selected    []bhruntime.ProjectResource
	resources   []bhruntime.ProjectResource
	inspectErr  error
	mutationErr error
}

func (r *brokerRetirementRuntime) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }
func (r *brokerRetirementRuntime) InspectProjectResources(_ context.Context, project string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	if r.inspectErr != nil {
		return nil, r.inspectErr
	}
	if project != r.project {
		return nil, errors.New("wrong project")
	}
	observed := []bhruntime.ProjectResource{}
	for _, resource := range r.resources {
		for _, owner := range expected {
			if resource == owner {
				observed = append(observed, resource)
			}
		}
	}
	return observed, nil
}
func (r *brokerRetirementRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return []bhruntime.RuntimeContainer{{Name: r.project + "-broker-1", Project: r.project, Service: ServiceName}}, nil
}
func (r *brokerRetirementRuntime) RetireOwnedProjectContainers(_ context.Context, project string, selected []bhruntime.ProjectResource) error {
	if project != r.project {
		return errors.New("wrong project")
	}
	r.selected = append([]bhruntime.ProjectResource{}, selected...)
	remaining := []bhruntime.ProjectResource{}
	for _, resource := range r.resources {
		remove := false
		for _, target := range selected {
			if target == resource {
				remove = true
			}
		}
		if !remove {
			remaining = append(remaining, resource)
		}
	}
	r.resources = remaining
	return r.mutationErr
}

func TestBrokerRetirementKeepsConsolidatedWorkloadBackendAndData(t *testing.T) {
	m := application.New("example", "dev", false, false, true)
	files := application.RuntimeFiles{Namespace: "isolated"}
	project := ProjectNameForRuntime(m, files)
	broker := bhruntime.ProjectResource{Kind: "container", Name: project + "-broker-1"}
	siblings := []bhruntime.ProjectResource{{Kind: "container", Name: project + "-web-1"}, {Kind: "container", Name: project + "-postgres-default-1"}, {Kind: "volume", Name: project + "_runtime-operations"}, {Kind: "network", Name: project + "-backend"}}
	r := &brokerRetirementRuntime{project: project, resources: append([]bhruntime.ProjectResource{broker}, siblings...)}
	result, err := RetireContainer(context.Background(), r, m, files)
	if err != nil || !reflect.DeepEqual(r.selected, []bhruntime.ProjectResource{broker}) || !reflect.DeepEqual(r.resources, siblings) || !reflect.DeepEqual(result.Removed, []bhruntime.ProjectResource{broker}) {
		t.Fatalf("broker retirement lost sibling resources: %+v %+v %v", r.resources, result, err)
	}
	if _, err := RetireContainer(context.Background(), r, m, files); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestBrokerRetirementRejectsForeignOwnerAndReportsInterruption(t *testing.T) {
	m := application.New("example", "dev", false, false, true)
	files := application.RuntimeFiles{Namespace: "isolated"}
	project := ProjectNameForRuntime(m, files)
	broker := bhruntime.ProjectResource{Kind: "container", Name: project + "-broker-1"}
	r := &brokerRetirementRuntime{project: project, inspectErr: bhruntime.ErrResourceOwnership}
	if _, err := RetireContainer(context.Background(), r, m, files); !errors.Is(err, bhruntime.ErrResourceOwnership) || len(r.selected) > 0 {
		t.Fatal("foreign broker was mutated")
	}
	interrupted := errors.New("interrupted")
	r = &brokerRetirementRuntime{project: project, resources: []bhruntime.ProjectResource{broker}, mutationErr: interrupted}
	result, err := RetireContainer(context.Background(), r, m, files)
	if !errors.Is(err, interrupted) || !reflect.DeepEqual(result.Removed, []bhruntime.ProjectResource{broker}) {
		t.Fatalf("partial result lost: %+v %v", result, err)
	}
	m.Services.Secrets = false
	r.selected = nil
	if _, err := RetireContainer(context.Background(), r, m, files); err == nil || len(r.selected) > 0 {
		t.Fatal("undeclared broker retirement accepted")
	}
}
