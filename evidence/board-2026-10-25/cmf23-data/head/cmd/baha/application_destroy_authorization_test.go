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

func TestApplicationDestructionRequiresCurrentOuterScopeAndSeparateDataConsent(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	registry := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	before, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, reset := range []bool{false, true} {
		p, err := buildApplicationDestructionPlan(t.Context(), r, reset)
		if err != nil {
			t.Fatal(err)
		}
		if p.Scope.Complete || p.Scope.ConfirmationsRequired != 2 || p.Scope.Attention != "Achtung" || p.Scope.Destructive != reset || p.Scope.Owner.Subject != r.DeploymentIdentity.DeploymentID || len(p.Materials) == 0 || len(p.Scope.Targets) != 1 {
			t.Fatal("selected owner/materials, unknown consequences or destructive scope lost")
		}
		leaf := p.Scope.Targets[0].PlanID
		invalid := []*application.ProviderChangeConsent{nil, {PlanID: p.Scope.PlanID, First: true}, {PlanID: leaf, First: true, Second: true, DeleteData: true}, {PlanID: "stale", First: true, Second: true, DeleteData: true}}
		if reset {
			invalid = append(invalid, &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true})
		}
		for _, consent := range invalid {
			_, err := authorizeApplicationDestruction(withProviderChangeConsent(t.Context(), consent), r, reset, io.Discard)
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Details == nil {
				t.Fatal("destruction bypassed current outer scope", err)
			}
		}
		ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: reset}
		if _, err := authorizeApplicationDestruction(withProviderChangeConsent(t.Context(), ack), r, reset, io.Discard); err != nil {
			t.Fatal("deliberately accepted unknown consequences became a policy veto", err)
		}
		after, err := os.ReadFile(registry)
		if err != nil || string(after) != string(before) {
			t.Fatal("planning/consent mutated original registry")
		}
	}
}

func TestApplicationDestructionConsentInvalidatedByRetainedPublicationChange(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	p, err := buildApplicationDestructionPlan(t.Context(), r, true)
	if err != nil {
		t.Fatal(err)
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState["recovery_reference"] = "changed-owned-recovery-reference"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationDestruction(withProviderChangeConsent(t.Context(), ack), r, true, io.Discard); err == nil {
		t.Fatal("changed protected owner retained old consent")
	}
}
