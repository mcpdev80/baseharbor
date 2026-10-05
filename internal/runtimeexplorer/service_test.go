package runtimeexplorer

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type fakeBackend struct {
	containers []runtimecontract.RuntimeContainer
	operated   []string
}

func (f *fakeBackend) Kind() runtimecontract.ProviderKind { return runtimecontract.ProviderDocker }
func (f *fakeBackend) ListRuntimeContainers(context.Context) ([]runtimecontract.RuntimeContainer, error) {
	return append([]runtimecontract.RuntimeContainer(nil), f.containers...), nil
}
func (f *fakeBackend) ContainerLogs(context.Context, string, *time.Time, int, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("line-1\nline-2\n")), nil
}
func (f *fakeBackend) OperateContainer(_ context.Context, id string, operation Operation, _ []string) (string, error) {
	f.operated = append(f.operated, id+":"+string(operation))
	for i := range f.containers {
		if f.containers[i].ID == id && operation == OperationStop {
			f.containers[i].Running = false
			f.containers[i].State = "exited"
		}
	}
	return "", nil
}

type fakeResolver struct{}

func (fakeResolver) ResolveContainer(_ context.Context, _ string, _ runtimecontract.ProviderKind, container runtimecontract.RuntimeContainer) (OwnershipEvidence, error) {
	if container.Project == "bh-demo" {
		return OwnershipEvidence{
			Ownership: OwnershipManaged,
			Relationship: Relationship{
				ApplicationID: "app-1",
				DeploymentID:  "dep-1",
				Application:   "demo",
				Environment:   "dev",
				Component:     container.Service,
			},
			Reconciliation: ReconciliationHint{
				PreferredOperation:            "application.reconcile",
				DirectMutationMayBeReconciled: true,
			},
		}, nil
	}
	return OwnershipEvidence{Ownership: OwnershipUnmanaged}, nil
}

func TestServiceListsStableManagedAndUnmanagedContainers(t *testing.T) {
	backend := &fakeBackend{containers: []runtimecontract.RuntimeContainer{
		{ID: "id-managed", Name: "demo-api-1", Project: "bh-demo", Service: "api", Running: true, State: "running", Health: "healthy"},
		{ID: "id-foreign", Name: "foreign", Running: true, State: "running"},
	}}
	service, err := NewService(backend, "local", fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	resources, err := service.List(context.Background(), ListRequest{Target: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(resources))
	}
	var managed Resource
	for _, resource := range resources {
		if resource.Ref.ResourceID == "id-managed" {
			managed = resource
		}
	}
	if managed.Ownership != OwnershipManaged || managed.Relationship.DeploymentID != "dep-1" || managed.Relationship.Component != "api" {
		t.Fatalf("managed relationship mismatch: %#v", managed)
	}
	if managed.Ref.ResourceID != "id-managed" {
		t.Fatalf("stable runtime id was not preserved: %#v", managed.Ref)
	}
}

func TestServiceRefusesMutationOfUnmanagedResource(t *testing.T) {
	backend := &fakeBackend{containers: []runtimecontract.RuntimeContainer{
		{ID: "id-foreign", Name: "foreign", Running: true, State: "running"},
	}}
	service, err := NewService(backend, "local", fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Operate(context.Background(), OperationRequest{
		Resource:  ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "id-foreign"},
		Operation: OperationStop,
	})
	if err == nil {
		t.Fatal("unmanaged runtime resource mutation unexpectedly succeeded")
	}
	if len(backend.operated) != 0 {
		t.Fatalf("backend mutated unmanaged resource: %#v", backend.operated)
	}
}

func TestServiceOperatesManagedResourceAndReturnsReconciliationHint(t *testing.T) {
	backend := &fakeBackend{containers: []runtimecontract.RuntimeContainer{
		{ID: "id-managed", Name: "demo-api-1", Project: "bh-demo", Service: "api", Running: true, State: "running"},
	}}
	service, err := NewService(backend, "local", fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Operate(context.Background(), OperationRequest{
		Resource:  ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "id-managed"},
		Operation: OperationStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Observed != "exited" || !result.Reconciliation.DirectMutationMayBeReconciled {
		t.Fatalf("unexpected operation result: %#v", result)
	}
}
