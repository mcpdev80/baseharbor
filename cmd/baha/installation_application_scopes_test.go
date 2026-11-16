package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationDestructionDelegatesOnlyDisplayedCurrentChildScope(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	p, err := buildRuntimeDestroyImpact(t.Context(), []targetDestroyInventory{{Target: r.Target}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Applications) != 1 || p.Applications[0].Scope.Owner.Subject != r.DeploymentIdentity.DeploymentID {
		t.Fatal("aggregate omitted actual application reset")
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	for _, invalid := range []*application.ProviderChangeConsent{nil, {PlanID: p.Scope.PlanID, First: true, Second: true}, {PlanID: "stale", First: true, Second: true, DeleteData: true}} {
		if _, err := withInstallationApplicationScopes(withProviderChangeConsent(t.Context(), invalid), p); err == nil {
			t.Fatal("missing/stale/data consent delegated destructive authority")
		}
	}
	ctx, err := withInstallationApplicationScopes(withProviderChangeConsent(t.Context(), ack), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(ctx, r, true, io.Discard); err != nil {
		t.Fatal("exact confirmed child refused", err)
	}
	if _, err := authorizeApplicationDestruction(ctx, r, false, io.Discard); err == nil {
		t.Fatal("reset authority substituted a different operation")
	}
	root := filepath.Join(r.DeploymentStateRoot, "state", "changed-material")
	if err := os.WriteFile(root, []byte("current owner changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(ctx, r, true, io.Discard); err == nil {
		t.Fatal("changed child owner reused aggregate consent")
	}
}

func TestInteractiveInstallationAcknowledgementsRemainBoundForDelegation(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	p, err := buildRuntimeDestroyImpact(t.Context(), []targetDestroyInventory{{Target: r.Target}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	recorded := &application.ProviderChangeConsent{}
	ctx := context.WithValue(t.Context(), runtimeDestroyAcknowledgementKey{}, recorded)
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	if _, err := authorizeRuntimeDestroyImpact(withProviderChangeConsent(ctx, ack), p, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := withInstallationApplicationScopes(withProviderChangeConsent(ctx, nil), p); err != nil {
		t.Fatal("actual acknowledged stages lost", err)
	}
}
