package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

func runtimeImpactFixture(t *testing.T) (runtimeexplorer.Resource, deployment.DeploymentRecord) {
	t.Helper()
	target := configureTestTarget(t)
	m := application.WithWorkloadComponents(application.New("runtime-owner", "dev", false, false, false), "api")
	record := registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
	ready := true
	return runtimeexplorer.Resource{ContractVersion: runtimeexplorer.ContractVersion, Ref: runtimeexplorer.ResourceRef{Provider: "docker", Target: target.Name, Kind: runtimeexplorer.KindContainer, ResourceID: "actual-selected-id"}, Ownership: runtimeexplorer.OwnershipManaged, Relationship: runtimeexplorer.Relationship{ApplicationID: m.ApplicationID, DeploymentID: record.Identity.DeploymentID, Application: m.Name, Environment: m.Environment, Component: "api"}, State: runtimeexplorer.ResourceState{Observed: "running", Ready: &ready}}, record
}

func TestRuntimeImpactRequiresTwoCurrentAcknowledgementsAndRetainsUnknownEffects(t *testing.T) {
	resource, record := runtimeImpactFixture(t)
	request := runtimeexplorer.OperationRequest{Resource: resource.Ref, Operation: runtimeexplorer.OperationStop}
	plan, err := planRuntimeMutation(resource, request.Operation)
	if err != nil || plan.Scope.Complete || plan.Scope.Attention != "Achtung" || plan.Scope.ConfirmationsRequired != 2 || len(plan.Materials) == 0 || len(plan.Functions) == 0 {
		t.Fatalf("missing actual owners or honest consequences: %+v %v", plan, err)
	}
	for _, ack := range []application.ProviderChangeConsent{{}, {PlanID: plan.Scope.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}} {
		_, err := (runtimeMutationAuthority{}).AuthorizeRuntimeMutation(withProviderChangeConsent(context.Background(), &ack), resource, request)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatal("missing or stale acknowledgement bypassed plan", err)
		}
	}
	_, missingErr := (runtimeMutationAuthority{}).AuthorizeRuntimeMutation(withProviderChangeConsent(t.Context(), nil), resource, request)
	var missing *machine.Error
	if !errors.As(missingErr, &missing) || missing.Code != machine.ErrorApprovalRequired {
		t.Fatalf("omitted consent has wrong interface semantics: %v", missingErr)
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	verify, err := (runtimeMutationAuthority{}).AuthorizeRuntimeMutation(withProviderChangeConsent(context.Background(), &ack), resource, request)
	if err != nil {
		t.Fatal("informed developer blocked", err)
	}
	stopped := resource
	ready := false
	stopped.State = runtimeexplorer.ResourceState{Observed: "exited", Ready: &ready}
	receipt, err := verify(t.Context(), stopped, nil)
	if err != nil || receipt.Status != "partial" || !receipt.Owner.Verified || receipt.Owner.Disposition != "retained" {
		t.Fatalf("runtime observation fabricated complete function recovery: %+v %v", receipt, err)
	}
	current, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil || current.Observed.State != record.Observed.State {
		t.Fatal("direct runtime operation silently rewrote deployment intent")
	}
}

func TestRuntimeImpactRejectsStaleOwnerAndForeignLinkedRecovery(t *testing.T) {
	resource, record := runtimeImpactFixture(t)
	plan, err := planRuntimeMutation(resource, runtimeexplorer.OperationStop)
	if err != nil {
		t.Fatal(err)
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	record.Applied.GeneratedState = map[string]string{"recovery_reference": "retained/changed"}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if _, err := (runtimeMutationAuthority{}).AuthorizeRuntimeMutation(withProviderChangeConsent(t.Context(), &ack), resource, runtimeexplorer.OperationRequest{Resource: resource.Ref, Operation: runtimeexplorer.OperationStop}); err == nil {
		t.Fatal("changed recovery owner retained stale consent")
	}
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "deployment.json")
	linked := filepath.Join(t.TempDir(), "foreign-link")
	if err := os.Link(path, linked); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(linked)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planRuntimeMutation(resource, runtimeexplorer.OperationStop); err == nil {
		t.Fatal("multi-linked recovery owner granted resource authority")
	}
	after, _ := os.ReadFile(linked)
	if string(before) != string(after) {
		t.Fatal("rejected owner changed foreign linked material")
	}
}

func TestRuntimeImpactReportsPartialCommandFailureAndUnhealthyRunningStop(t *testing.T) {
	resource, _ := runtimeImpactFixture(t)
	plan, err := planRuntimeMutation(resource, runtimeexplorer.OperationStop)
	if err != nil {
		t.Fatal(err)
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	verify, err := (runtimeMutationAuthority{}).AuthorizeRuntimeMutation(withProviderChangeConsent(t.Context(), &ack), resource, runtimeexplorer.OperationRequest{Resource: resource.Ref, Operation: runtimeexplorer.OperationStop})
	if err != nil {
		t.Fatal(err)
	}
	ready := false
	unhealthy := resource
	unhealthy.State.Ready = &ready
	unhealthy.State.Health = "unhealthy"
	if receipt, err := verify(t.Context(), unhealthy, nil); err == nil || receipt == nil {
		t.Fatal("unhealthy running container mistaken for verified stop")
	}
	stopped := unhealthy
	stopped.State.Observed = "exited"
	if receipt, err := verify(t.Context(), stopped, errors.New("interrupted after native stop")); err == nil || receipt == nil || receipt.Status != "partial" {
		t.Fatal("command failure lost actual partial receipt")
	}
}
