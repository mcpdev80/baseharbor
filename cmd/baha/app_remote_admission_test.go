package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteUnsupportedProviderDisclosesCurrentImpactBeforeTechnicalDenial(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = machineNoninteractiveContext(withAssumeYes(withCoreAuthority(ctx, core), true))
	root, err := deployment.TargetStateRoot(node.Name)
	if err != nil {
		t.Fatal(err)
	}
	m := application.WithWorkloadComponents(application.New("remote-impact", "dev", false, false, false), "api")
	m.Services.Identity = true
	r := resolvedApplication{Target: node, TargetStateRoot: root, Manifest: m, Store: application.Store{Root: t.TempDir()}}
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(root, "provider-registry.json")
	before, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	initialEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	initialMembers := map[string]bool{}
	for _, entry := range initialEntries {
		initialMembers[entry.Name()] = true
	}
	// Select intentional loss of the retained Identity capability while adding
	// another unsupported remote capability. Neither decision may hide impact.
	r.Manifest.Services.Identity = false
	r.Manifest.Services.MessagingQueue = true
	plan, err := buildResolvedApplicationPlan(ctx, r)
	if err != nil || plan.ProviderPlan.Impact == nil {
		t.Fatalf("concrete remote consequence plan unavailable: %v", err)
	}
	impact := plan.ProviderPlan.Impact
	if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("unsupported remote change lacks current two-stage consequence plan")
	}
	for _, consent := range []*application.ProviderChangeConsent{
		nil,
		{PlanID: impact.PlanID, First: true},
		{PlanID: "stale", First: true, Second: true},
		{PlanID: impact.PlanID, First: true, Second: true},
	} {
		err := executeRemoteApplicationApply(withProviderChangeConsent(ctx, consent), r, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) {
			t.Fatalf("untyped remote admission result: %v", err)
		}
		if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
			if failure.Code != machine.ErrorCapabilityMissing {
				t.Fatalf("acknowledgement fabricated remote support or reached transport: %v", err)
			}
		} else {
			if failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict {
				t.Fatalf("technical denial hid unacknowledged remote consequences: %v", err)
			}
			actual, ok := failure.Details.(application.ProviderImpactPlan)
			if !ok || actual.PlanID != impact.PlanID {
				t.Fatal("admission did not return the concrete current provider impact")
			}
		}
		after, err := os.ReadFile(registry)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("unsupported remote request changed retained provider bindings")
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !initialMembers[entry.Name()] && entry.Name() != ".installation.lock" {
				t.Fatalf("unsupported remote request staged provider, deployment or material member %q", entry.Name())
			}
		}
	}
}
