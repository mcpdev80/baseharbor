package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestUnsupportedIdentityReplacementShowsConsequencesBeforeExecutionAdmission(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	r.Manifest.Services.Identity = true
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "shared")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "application")
	plan, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil {
		t.Fatalf("selection was vetoed before consequence disclosure: %v", err)
	}
	impact := plan.ProviderPlan.Impact
	if impact == nil || impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("unknown replacement consequences lacked current two-stage plan")
	}
	selected := false
	for _, item := range impact.Impacts {
		for _, effect := range item.Effects {
			if effect.Aspect == "consumer-rebinding" {
				selected = true
			}
			if effect.Aspect == "data-continuity" {
				t.Fatal("Identity was presented as SQL storage migration")
			}
		}
	}
	if !selected {
		t.Fatal("selected Identity replacement effects were omitted")
	}
	for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
		err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) {
			t.Fatal("unimplemented replacement was executable")
		}
		if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
			if failure.Code != machine.ErrorUnsupported || failure.CauseCode != "provider_binding_migration_required" || failure.Details == nil {
				t.Fatalf("missing adapter was not an explicit execution failure with the plan: %v", err)
			}
		} else if failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict {
			t.Fatalf("technical admission hid the current acknowledgement requirement: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, original) {
			t.Fatal("plan or denied execution changed the canonical original binding")
		}
	}
}
