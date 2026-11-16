package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRestoreRefusesBeforeRuntimeMutationAndBindsArchiveOwnerAndDataDecision(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	if err := resetRestoreTarget(t.Context(), nil, bhruntime.Files{}, r); machine.Classify(err).Code != machine.ErrorApprovalRequired {
		t.Fatal("lower restore reset bypassed authority", err)
	}
	data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
	p, err := buildApplicationRestorePlan(t.Context(), r, data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Scope.Owner.Owner != "application-restore" || !p.Scope.Destructive || p.Scope.Complete || p.Scope.ConfirmationsRequired != 2 || p.Scope.Attention != "Achtung" || len(p.Materials) == 0 {
		t.Fatal("restore did not inventory destructive/unknown consequences")
	}
	registry := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	before, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range []*application.ProviderChangeConsent{nil,
		{PlanID: p.Scope.PlanID, First: true},
		{PlanID: p.Scope.PlanID, First: true, Second: true},
		{PlanID: p.Scope.Targets[0].PlanID, First: true, Second: true, DeleteData: true},
	} {
		// nil native executor is deliberate: consent must fail before preparing
		// providers, routes, recovery state or deleting anything.
		err := restoreApplicationState(withProviderChangeConsent(t.Context(), ack), r.Store, io.Discard, r, nil, data)
		var failure *machine.Error
		if !errors.As(err, &failure) || (failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict) || failure.Details == nil {
			t.Fatal("restore admission bypassed explicit consequences/data consent", err)
		}
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err != nil {
		t.Fatal("explicitly acknowledged restore was vetoed", err)
	}
	owner := filepath.Join(r.DeploymentStateRoot, "state", "restore-owned-material")
	if err := os.WriteFile(owner, []byte("actual changed recovery owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err == nil {
		t.Fatal("changed retained material reused archive consent")
	}
	if err := os.Remove(owner); err != nil {
		t.Fatal(err)
	}
	data.archiveRevision = strings.Repeat("b", 64)
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err == nil {
		t.Fatal("different validated archive reused consent")
	}
	data.archiveRevision = strings.Repeat("a", 64)
	destruction, err := buildApplicationDestructionPlan(t.Context(), r, true)
	if err != nil {
		t.Fatal(err)
	}
	ack.PlanID = destruction.Scope.PlanID
	if err := authorizeApplicationRestore(withProviderChangeConsent(t.Context(), ack), r, data, io.Discard); err == nil {
		t.Fatal("destroy consent authorized restore")
	}
	after, err := os.ReadFile(registry)
	if err != nil || string(after) != string(before) {
		t.Fatal("restore planning or denial mutated canonical registry")
	}
}

func TestRestoreJSONAndEnvironmentSelectorsReachCanonicalValidation(t *testing.T) {
	command := appRestoreCommandWithRecoveryMetadata(application.Store{})
	err := command.Run(t.Context(), []string{"missing.bhbackup", "--password-file", filepath.Join(t.TempDir(), "missing-password"), "--environment", "dev", "--json"}, io.Discard, io.Discard)
	if err == nil || strings.Contains(err.Error(), "unknown option") || !strings.Contains(err.Error(), "inspect backup password file") {
		t.Fatal("JSON/environment presentation bypassed canonical restore validation", err)
	}
}
