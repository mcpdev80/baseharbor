package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestCoreProviderMaterialBindsActualProtectedBytesWithoutClaimingDelivery(t *testing.T) {
	files := coreStopFixture(t)
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target}
	instance := application.RuntimeStorageDependency(target.Name)
	observe := func() application.ProviderImpactFact {
		fact := application.ProviderImpactFact{Realization: application.ProviderRealization{Ancillary: true, Owner: filepath.Dir(files.Compose)}}
		observeExistingCoreProviderMaterial(resolved, instance, &fact)
		return fact
	}
	path := filepath.Join(filepath.Dir(files.Compose), "protected-recovery-material")
	secret := "test-private-recovery-value"
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	prior := observe()
	if len(prior.Materials) == 0 || prior.InventoryComplete || prior.Realization.Verified || prior.Realization.Supported {
		t.Fatal("protected files incorrectly became working provider/consumer evidence")
	}
	encoded, _ := json.Marshal(prior)
	if strings.Contains(string(encoded), secret) {
		t.Fatal("material observation disclosed secret contents")
	}
	revision := func(fact application.ProviderImpactFact) string {
		value, err := applicationImpactFileRevision(application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{instance.ID: fact}})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := revision(prior)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test-private-recovery-other"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if revision(observe()) == before {
		t.Fatal("changed same-size, same-mtime Core material retained plan revision")
	}
}

func TestCoreProviderMaterialDoesNotAdoptUnrelatedSQLOrModifiedCore(t *testing.T) {
	files := coreStopFixture(t)
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target}
	fact := application.ProviderImpactFact{Realization: application.ProviderRealization{Owner: "retained-owner"}}
	instance := capability.ProviderInstance{ID: "unrelated-sql", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	observeExistingCoreProviderMaterial(resolved, instance, &fact)
	if len(fact.Materials) != 0 || len(fact.Realization.Locations) != 0 {
		t.Fatal("unrelated PostgreSQL acquired Core material")
	}
	instance = capability.ProviderInstance{ID: "unrelated-secrets", Provider: capability.OpenBao, Scope: capability.ScopeShared, SharingBoundary: "foreign-module", Ownership: capability.OwnershipBaseHarbor}
	observeExistingCoreProviderMaterial(resolved, instance, &fact)
	if len(fact.Materials) != 0 {
		t.Fatal("unsupported separate OpenBao module acquired Core material")
	}
	if err := os.WriteFile(files.Compose, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	instance = application.RuntimeStorageDependency(target.Name)
	fact.Realization.Ancillary = true
	observeExistingCoreProviderMaterial(resolved, instance, &fact)
	if len(fact.Materials) != 0 || fact.Realization.Verified {
		t.Fatal("modified Core definition acquired canonical ownership")
	}
}
