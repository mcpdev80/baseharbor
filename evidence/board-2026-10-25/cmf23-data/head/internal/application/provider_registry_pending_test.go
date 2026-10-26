package application

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestPendingProviderOwnersPreserveAppliedConsumersUntilVerifiedRelease(t *testing.T) {
	root := t.TempDir()
	before := New("demo", "dev", false, false, false)
	before.Services.Identity = true
	if err := ReconcileReferenceProviderRegistryAt(root, before); err != nil {
		t.Fatal(err)
	}
	store, _ := referenceProviderRegistryStoreAt(root)
	applied, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desired := before
	desired.Services.Identity = false
	desired.Services.Cache = true
	for attempt := 0; attempt < 2; attempt++ {
		if err := RetainPendingProviderOwnersAt(root, desired); err != nil {
			t.Fatal(err)
		}
		pending, err := store.Load()
		if err != nil || !reflect.DeepEqual(applied.Bindings, pending.Bindings) {
			t.Fatalf("attempt %d released applied consumers: %#v %v", attempt, pending, err)
		}
		if len(pending.Instances) != len(applied.Instances)+1 {
			t.Fatalf("new owner missing or duplicated: %#v", pending.Instances)
		}
	}
	// Only the verified lifecycle's final reconciliation publishes the new
	// binding. Both retained realization owners remain available for recovery.
	if err := ReconcileReferenceProviderRegistryAt(root, desired); err != nil {
		t.Fatal(err)
	}
	verified, err := store.Load()
	if err != nil || len(verified.Bindings) != 1 || verified.Bindings[0].Resource.Provider != capability.ProviderValkey || len(verified.Instances) != 2 {
		t.Fatalf("verified registry: %#v %v", verified, err)
	}
}

func TestPendingProviderOwnersDoNotReplaceAppliedPlacement(t *testing.T) {
	root := t.TempDir()
	m := New("demo", "dev", true, false, false)
	if err := ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	store, _ := referenceProviderRegistryStoreAt(root)
	applied, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	if err := RetainPendingProviderOwnersAt(root, m); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Load()
	if err != nil || !reflect.DeepEqual(applied.Bindings, pending.Bindings) || len(pending.Instances) != 2 {
		t.Fatalf("pending replacement changed applied placement: %#v %v", pending, err)
	}
}

func TestPendingProviderOwnersRejectInvalidResourcesWithoutPartialWrite(t *testing.T) {
	root := t.TempDir()
	m := New("demo", "dev", false, false, false)
	if err := ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, providerRegistryFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m.Services.Identity = true
	foreign := capability.Resource{Application: "other", Kind: capability.KeyValue, Provider: capability.ProviderValkey, Name: "default"}
	if err := RetainPendingProviderOwnersAt(root, m, foreign); err == nil {
		t.Fatal("foreign resource accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("failed projection changed authoritative registry: %v", err)
	}
}
