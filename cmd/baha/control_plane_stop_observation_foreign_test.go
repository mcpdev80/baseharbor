package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// controlPlaneStopObservationProvider drives observeControlPlaneStopWithProvider.
// It implements the exact subset of RuntimeProvider the control-plane seam
// consumes: InspectProjectResources, ListRuntimeContainers, the optional
// ConfigJSONProjectFilesEnv render capability, and ProjectServiceImageIdentity.
type controlPlaneStopObservationProvider struct {
	bhruntime.RuntimeProvider
	containers []bhruntime.RuntimeContainer
	inspected  []bhruntime.ProjectResource
	rendered   string
	renderErr  error
	image      bhruntime.ImageIdentity
	imageErr   error
}

// Kind is the only embedded method the control-plane seam calls that the embed
// (a nil interface) cannot answer without panicking.

func (r *controlPlaneStopObservationProvider) Kind() bhruntime.ProviderKind {
	return bhruntime.ProviderDocker
}

func (r *controlPlaneStopObservationProvider) InspectProjectResources(_ context.Context, _ string, resources []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	r.inspected = append([]bhruntime.ProjectResource{}, resources...)
	return resources, nil
}

func (r *controlPlaneStopObservationProvider) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return r.containers, nil
}

func (r *controlPlaneStopObservationProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return r.rendered, r.renderErr
}

func (r *controlPlaneStopObservationProvider) ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error) {
	return r.image, r.imageErr
}

// observeControlPlaneStopPlanFixture builds a stopped-plan for one owned service
// with one pinned native container_name so the cross-project contract is
// exercised on the production path (not only the matcher).
func controlPlaneStopPlanFixture(project, service, pinnedName string, containers []bhruntime.RuntimeContainer) (deployment.ResolvedTarget, *controlPlaneStopPlan) {
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker", AccessProvider: "local"}
	p := &controlPlaneStopPlan{
		Target:   target.Name,
		Project:  project,
		Services: []string{service},
		Resources: []bhruntime.ProjectResource{
			{Kind: "container", Name: pinnedName},
			{Kind: "volume", Name: project + "-data"},
		},
	}
	return target, p
}

// TestObserveControlPlaneStopRejectsForeignPinnedReplica proves on the
// production control-plane-stop observation path (not only the matcher) that a
// runtime inventory which also carries a FOREIGN-project row with the exact same
// pinned native name and the same service label never legitimizes that foreign
// container: the foreign row must not be accepted as an owned replica, must not
// be included in the observed/mutated set, and the same-name foreign row must
// not make the own pinned container fail closed either (0427-128 r6, recipe
// point 1; spiegelbildlich zu TestRetireOwnedServicesRejectsForeignPinnedReplica).
func TestObserveControlPlaneStopRejectsForeignPinnedReplica(t *testing.T) {
	const project = "linkding"
	const service = "web"
	const pinned = "linkding-web"

	// Valid owned pinned replica: the control-plane observation admits it and
	// never rejects on the canonical-only guard.
	_, plan := controlPlaneStopPlanFixture(project, service, pinned,
		[]bhruntime.RuntimeContainer{{Name: pinned, Project: project, Service: service, Running: false}})
	provider := &controlPlaneStopObservationProvider{
		containers: []bhruntime.RuntimeContainer{{Name: pinned, Project: project, Service: service, Running: false}},
		rendered:   `{"services":{"web":{"container_name":"linkding-web"}}}`,
	}
	if err := observeControlPlaneStopWithProvider(context.Background(), provider, plan); err != nil {
		t.Fatalf("valid owned pinned replica rejected open: %v", err)
	}
	if !plan.RuntimeObserved {
		t.Fatal("control-plane observation did not record the owned pinned replica")
	}

	// Cross-project: the full runtime inventory additionally carries a
	// foreign-project row with the exact same pinned native name and service.
	// The control-plane observation must neither accept that row as an owned
	// replica nor include it in the observed set, and must not let it disturb
	// the own pinned container.
	_, plan = controlPlaneStopPlanFixture(project, service, pinned,
		[]bhruntime.RuntimeContainer{
			{Name: pinned, Project: project, Service: service, Running: false},
			{Name: pinned, Project: "foreign", Service: service, Running: true},
		})
	provider = &controlPlaneStopObservationProvider{
		containers: []bhruntime.RuntimeContainer{
			{Name: pinned, Project: project, Service: service, Running: false},
			{Name: pinned, Project: "foreign", Service: service, Running: true},
		},
		rendered: `{"services":{"web":{"container_name":"linkding-web"}}}`,
	}
	if err := observeControlPlaneStopWithProvider(context.Background(), provider, plan); err != nil {
		t.Fatalf("cross-project pinned replica failed the owned-path check: %v", err)
	}
	// The foreign row must not enter the observed processes; only the owned
	// pinned replica is observed.
	if len(plan.ObservedProcesses) != 1 || plan.ObservedProcesses[0].Name != pinned || plan.ObservedProcesses[0].Service != service {
		t.Fatalf("foreign pinned row leaked into the observed set: %+v", plan.ObservedProcesses)
	}
	// The matcher contract stays: the same pinned native name under a foreign
	// project is never an owned replica of the owning project's planned set.
	if bhruntime.ContainerNameInPlannedSet(bhruntime.ProviderDocker, bhruntime.RuntimeContainer{Name: pinned, Project: "foreign", Service: service}, project, []string{pinned}) {
		t.Fatal("foreign project pinned name must not match the owning project planned set")
	}

	// Foreign-only inventory: only the foreign same-name row is live. The
	// production observation must never treat it as an owned replica: the owning
	// project's planned pinned name is not admitted from a foreign-project row,
	// so nothing owned is observed (fail-closed) rather than the foreign
	// container entering the observed/mutated set.
	_, plan = controlPlaneStopPlanFixture(project, service, pinned,
		[]bhruntime.RuntimeContainer{{Name: pinned, Project: "foreign", Service: service, Running: true}})
	provider = &controlPlaneStopObservationProvider{
		containers: []bhruntime.RuntimeContainer{{Name: pinned, Project: "foreign", Service: service, Running: true}},
		rendered:   `{"services":{"web":{"container_name":"linkding-web"}}}`,
	}
	if err := observeControlPlaneStopWithProvider(context.Background(), provider, plan); err != nil {
		t.Fatalf("foreign-only pinned row raised an unexpected error: %v", err)
	}
	if len(plan.ObservedProcesses) != 0 {
		t.Fatalf("foreign-only pinned row was accepted as an owned replica into the observed set: %+v", plan.ObservedProcesses)
	}
}
