package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestInstallationPlanDisclosesUnregisteredLocalStateWithoutInventingRuntimeOwner(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	inventories := []targetDestroyInventory{{Target: r.Target}}
	before, err := buildRuntimeDestroyImpact(t.Context(), inventories, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	id, err := deployment.NewDeploymentIdentity("local", application.MustNewApplicationID(), "unfinished-preview", "dev")
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	after, err := buildRuntimeDestroyImpact(t.Context(), inventories, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if after.Scope.PlanID == before.Scope.PlanID || after.Scope.Complete || after.Scope.Attention != "Achtung" || after.Scope.ConfirmationsRequired != 2 {
		t.Fatal("new unknown local state was not bound/disclosed")
	}
	if len(after.Applications) != 1 || after.Applications[0].Scope.Owner.Subject != r.DeploymentIdentity.DeploymentID {
		t.Fatal("unregistered state became runtime destruction authority")
	}
	found := false
	for _, item := range after.Discovery {
		if item.Resource == "deployment "+id.DeploymentID {
			found = item.Status == "SKIPPED" && item.Diagnostic == "unregistered-deployment-state" && item.Target == id.Target
		}
	}
	if !found {
		t.Fatal("concrete unregistered state warning missing from displayed plan")
	}
	old := &application.ProviderChangeConsent{PlanID: before.Scope.PlanID, First: true, Second: true, DeleteData: true}
	if _, err := authorizeRuntimeDestroyImpact(withProviderChangeConsent(t.Context(), old), after, true, io.Discard); err == nil {
		t.Fatal("new unknown local state reused old consent")
	}
	if _, err := os.Stat(filepath.Join(root, "deployment.json")); !os.IsNotExist(err) {
		t.Fatal("planning manufactured owner")
	}
	if _, err := os.Stat(filepath.Join(root, "state")); err != nil {
		t.Fatal("planning deleted unregistered state")
	}
}
