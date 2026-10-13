package main

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type selectedCoreDestroyFake struct {
	resources          []bhruntime.ProjectResource
	containers         []bhruntime.RuntimeContainer
	deleteCalls        int
	stopErr, deleteErr error
}

func (p *selectedCoreDestroyFake) Kind() bhruntime.ProviderKind { return bhruntime.ProviderDocker }
func (p *selectedCoreDestroyFake) InspectProjectResources(_ context.Context, _ string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	var result []bhruntime.ProjectResource
	for _, actual := range p.resources {
		for _, want := range expected {
			if actual == want {
				result = append(result, actual)
			}
		}
	}
	return result, nil
}
func (p *selectedCoreDestroyFake) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return append([]bhruntime.RuntimeContainer{}, p.containers...), nil
}
func (p *selectedCoreDestroyFake) StopProjectFilesSelected(_ context.Context, _ string, _ string, _ map[string]string, services []string, _ ...string) error {
	if p.stopErr != nil {
		return p.stopErr
	}
	for i := range p.containers {
		for _, service := range services {
			if p.containers[i].Service == service {
				p.containers[i].Running, p.containers[i].State = false, "exited"
			}
		}
	}
	return nil
}
func (p *selectedCoreDestroyFake) RetireOwnedProjectContainers(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	p.remove(selected)
	return nil
}
func (p *selectedCoreDestroyFake) DestroyOwnedProjectResources(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	p.deleteCalls++
	if p.deleteErr != nil {
		return p.deleteErr
	}
	p.remove(selected)
	return nil
}
func (p *selectedCoreDestroyFake) remove(selected []bhruntime.ProjectResource) {
	for _, want := range selected {
		for i := 0; i < len(p.resources); i++ {
			if p.resources[i] == want {
				p.resources = append(p.resources[:i], p.resources[i+1:]...)
				i--
			}
		}
		if want.Kind == "container" {
			for i := 0; i < len(p.containers); i++ {
				if p.containers[i].Name == want.Name {
					p.containers = append(p.containers[:i], p.containers[i+1:]...)
					i--
				}
			}
		}
	}
}

func coreDestroyFakeFixture(t *testing.T, deleteData bool) (selectedCoreDestroyPlan, *selectedCoreDestroyFake) {
	t.Helper()
	coreStopFixture(t)
	p, err := buildSelectedCoreDestroyPlan(withSelectedCoreDataDeletion(t.Context(), deleteData))
	if err != nil {
		t.Fatal(err)
	}
	fake := &selectedCoreDestroyFake{resources: append([]bhruntime.ProjectResource{}, p.Owner.Resources...)}
	for _, service := range p.Owner.Services {
		fake.containers = append(fake.containers, bhruntime.RuntimeContainer{ID: service + "-id", Name: p.Owner.Project + "-" + service + "-1", Project: p.Owner.Project, Service: service, Running: true, State: "running"})
	}
	fake.containers = append(fake.containers, bhruntime.RuntimeContainer{ID: "sibling-id", Name: p.Owner.Project + "-sibling-1", Project: p.Owner.Project, Service: "sibling", Running: true, State: "running"})
	return p, fake
}

func TestSelectedCoreDestroyPreservesDataSiblingAndOwnersAndAllowsRetry(t *testing.T) {
	p, fake := coreDestroyFakeFixture(t, false)
	if p.Impact.Scope.Destructive || p.Impact.Scope.ConfirmationsRequired != 2 {
		t.Fatal("runtime and data destruction are conflated")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := executeSelectedCoreDestroy(t.Context(), p, fake); err != nil {
			t.Fatal(err)
		}
		if fake.deleteCalls != 0 || len(fake.containers) != 1 || fake.containers[0].ID != "sibling-id" || !fake.containers[0].Running {
			t.Fatal("retirement changed data or sibling")
		}
		for _, volume := range p.Volumes {
			present := false
			for _, actual := range fake.resources {
				present = present || actual == volume
			}
			if !present {
				t.Fatal("lost retained volume")
			}
		}
	}
}

func TestSelectedCoreDeleteDataRequiresFreshIndependentConsent(t *testing.T) {
	p, _ := coreDestroyFakeFixture(t, false)
	deleted, err := buildSelectedCoreDestroyPlan(withSelectedCoreDataDeletion(t.Context(), true))
	if err != nil || p.Impact.Scope.PlanID == deleted.Impact.Scope.PlanID || !deleted.Impact.Scope.Destructive {
		t.Fatal("data selection does not invalidate consent", err)
	}
	ack := &application.ProviderChangeConsent{PlanID: deleted.Impact.Scope.PlanID, First: true, Second: true}
	if _, err := authorizeRuntimeDestroyImpact(withProviderChangeConsent(t.Context(), ack), deleted.Impact, true, io.Discard); err == nil {
		t.Fatal("missing data consent accepted")
	}
}

func TestSelectedCoreDeleteFailureRetainsOwnersAndRetriesOnlyRemainingResources(t *testing.T) {
	p, fake := coreDestroyFakeFixture(t, true)
	fake.deleteErr = errors.New("volume is in use")
	if _, err := executeSelectedCoreDestroy(t.Context(), p, fake); err == nil || fake.deleteCalls != 1 {
		t.Fatal("volume failure reported success", err)
	}
	if _, err := os.Stat(p.Owner.inventory.Files.Compose); err != nil {
		t.Fatal("lost retry owner", err)
	}
	fake.deleteErr = nil
	if _, err := executeSelectedCoreDestroy(t.Context(), p, fake); err != nil {
		t.Fatal("retry failed", err)
	}
	for _, actual := range fake.resources {
		if actual.Kind == "volume" || actual.Kind == "container" {
			t.Fatal("selected resource remains")
		}
	}
	if len(fake.containers) != 1 || !fake.containers[0].Running {
		t.Fatal("data deletion changed sibling")
	}
}

func TestSelectedCoreStopFailureNeverAttemptsVolumeDeletion(t *testing.T) {
	p, fake := coreDestroyFakeFixture(t, true)
	fake.stopErr = errors.New("stop interrupted")
	if _, err := executeSelectedCoreDestroy(t.Context(), p, fake); err == nil || fake.deleteCalls != 0 {
		t.Fatal("stop failure permitted data deletion", err)
	}
}

func TestActiveCoreConsumerIsConsequenceInsteadOfPermanentDestroyVeto(t *testing.T) {
	coreStopFixture(t)
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("active-core-consumer", "dev", true, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	if err := application.CheckControlPlaneDestroySafeAt(root); err == nil {
		t.Fatal("fixture has no active consumer")
	}
	p, err := buildSelectedCoreDestroyPlan(t.Context())
	if err != nil {
		t.Fatal("active consumer blocked deliberate Core removal", err)
	}
	if p.Impact.Scope.Complete || p.Impact.Scope.ConfirmationsRequired != 2 || len(p.Impact.Core[0].Functions) < 2 {
		t.Fatal("active consequences missing", p.Impact)
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Impact.Scope.PlanID, First: true, Second: true}
	if proceed, err := authorizeRuntimeDestroyImpact(withProviderChangeConsent(t.Context(), ack), p.Impact, true, io.Discard); err != nil || !proceed {
		t.Fatal("fully acknowledged active consumer remains vetoed", err)
	}
}
