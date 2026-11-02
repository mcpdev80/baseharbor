package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestPersistControlPlaneOpenBaoReference(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	target := configureTestTarget(t)
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	for i := 0; i < 2; i++ {
		if err := persistControlPlaneOpenBaoReference(context.Background()); err != nil {
			t.Fatalf("registration %d: %v", i, err)
		}
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("worker", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithRuntimePermission(m, string(capability.MetricsV1.ID), []string{"api"}, "runtime.get")
	roots, err := application.RuntimeDependencyRoots(m, registry)
	if err != nil || len(roots) != 1 || roots[0] != "openbao/control-plane" {
		t.Fatalf("persisted broker dependency roots: %v, %v", roots, err)
	}
	if len(registry.Instances) != 1 || len(registry.Bindings) != 0 {
		t.Fatalf("unexpected duplicate or synthetic binding: %+v", registry)
	}
	if err := application.ReleaseApplicationProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	registry, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.RuntimeDependencyRoots(m, registry); err != nil {
		t.Fatalf("application release removed shared control-plane reference: %v", err)
	}
}

func TestPersistControlPlaneRejectsIncompatibleReference(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	target := configureTestTarget(t)
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	if err := store.Update(func(registry *capability.Registry) error {
		return registry.Register(capability.ProviderInstance{ID: "openbao/control-plane", Provider: capability.OpenBao, Scope: capability.ScopeExternal, Ownership: capability.OwnershipExternal, Reference: "untrusted"})
	}); err != nil {
		t.Fatal(err)
	}
	if err := persistControlPlaneOpenBaoReference(context.Background()); err == nil {
		t.Fatal("accepted an incompatible control-plane registry identity")
	}
}

func TestPersistControlPlaneRejectsMismatchedSharingBoundary(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	target := configureTestTarget(t)
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	if err := store.Update(func(registry *capability.Registry) error {
		return registry.Register(capability.ProviderInstance{
			ID: "openbao/control-plane", Provider: capability.OpenBao,
			Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor,
			SharingBoundary: "other-tenant",
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := persistControlPlaneOpenBaoReference(context.Background()); err == nil {
		t.Fatal("accepted a control-plane reference with a different sharing boundary")
	}
}
