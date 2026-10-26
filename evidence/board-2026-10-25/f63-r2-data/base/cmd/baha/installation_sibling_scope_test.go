package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestInstallationChildScopeAfterEarlierSiblingRemoval(t *testing.T) {
	first := retainedNativeBackendFixture(t)
	root := t.TempDir()
	t.Chdir(root)
	secondManifest := application.WithWorkloadComponents(application.New("second", "dev", false, false, false), "api")
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, secondManifest)
	second, err := resolveApplication(t.Context(), application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(t.Context(), second, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(second.TargetStateRoot, second.Manifest); err != nil {
		t.Fatal(err)
	}
	p, err := buildRuntimeDestroyImpact(t.Context(), []targetDestroyInventory{{Target: first.Target}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Applications) != 2 {
		t.Fatal("missing actual sibling reset scopes")
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	ctx, err := withInstallationApplicationScopes(withProviderChangeConsent(t.Context(), ack), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(ctx, second, true, io.Discard); err != nil {
		t.Fatal("unchanged second child refused", err)
	}
	// Model only the authoritative state transitions of an already completed
	// earlier child; no native-runtime success is implied by this source fixture.
	if err := application.ReleaseApplicationProviderReferencesAt(first.TargetStateRoot, first.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := deployment.DeleteDeploymentRecord(first.DeploymentIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(ctx, second, true, io.Discard); err == nil {
		t.Fatal("changed global union silently reused old parent acknowledgement")
	}
	next, err := buildRuntimeDestroyImpact(t.Context(), []targetDestroyInventory{{Target: first.Target}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	next.Continuation = &installationDestroyContinuation{PreviousPlanID: p.Scope.PlanID, CompletedDeploymentIDs: []string{first.DeploymentIdentity.DeploymentID}}
	if next.Scope.PlanID == p.Scope.PlanID || len(next.Applications) != 1 {
		t.Fatal("remaining plan did not reflect completed child")
	}
	_, err = authorizeRuntimeDestroyImpact(withProviderChangeConsent(ctx, nil), next, true, io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired || failure.CauseCode != "impact-confirmation-required" {
		t.Fatal("remaining plan did not require new current acknowledgements", err)
	}
	if failure.Details.(runtimeDestroyImpact).Continuation.PreviousPlanID != p.Scope.PlanID {
		t.Fatal("missing explicit progress boundary")
	}
	for _, incomplete := range []*application.ProviderChangeConsent{{PlanID: next.Scope.PlanID, First: true}, {PlanID: next.Scope.PlanID, First: true, Second: true}} {
		if _, err := withInstallationApplicationScopes(withProviderChangeConsent(t.Context(), incomplete), next); err == nil {
			t.Fatal("remaining plan bypassed stages or data consent")
		}
	}
	ack = &application.ProviderChangeConsent{PlanID: next.Scope.PlanID, First: true, Second: true, DeleteData: true}
	current, err := withInstallationApplicationScopes(withProviderChangeConsent(t.Context(), ack), next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(current, second, true, io.Discard); err != nil {
		t.Fatal("explicit current remaining plan permanently vetoed cleanup", err)
	}
}
