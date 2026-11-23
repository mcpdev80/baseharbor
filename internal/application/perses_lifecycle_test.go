package application

import (
	"errors"
	"testing"
)

// persesTestManifest builds a minimal manifest that selects the observability
// management-ui surface with managed metrics sources (Prometheus) and the
// optional Perses realization, i.e. the valid precondition for lifecycle
// operations.
func persesTestManifest(perses bool) Manifest {
	m := Manifest{
		Version:     1,
		Name:        "perses-app",
		Environment: "dev",
		Workload:    WorkloadConfig{Components: []string{"workload"}},
		Metrics:     MetricsRequirements{Sources: []MetricsSourceRequirement{{Name: "workload", Service: "workload", Port: 9090, Path: "/metrics"}}},
	}
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	return m
}

// TestPersesLifecycleUnauthorizedIsFailClosed asserts that an unauthorized caller
// receives a *PersesLifecycleError and no plan, for every action. This proves
// the ownership/authorization guard before any manifest mutation.
func TestPersesLifecycleUnauthorizedIsFailClosed(t *testing.T) {
	for _, action := range []PersesLifecycleAction{PersesLifecycleEnable, PersesLifecycleDisable, PersesLifecycleRemove} {
		m := persesTestManifest(false)
		plan, err := ResolvePersesLifecycle(m, action, action == PersesLifecycleEnable, false, true)
		if err == nil {
			t.Fatalf("action %q: expected fail-closed unauthorized error, got plan", action)
		}
		var lifecycErr *PersesLifecycleError
		if !errors.As(err, &lifecycErr) {
			t.Fatalf("action %q: expected *PersesLifecycleError, got %T: %v", action, err, err)
		}
		if plan.RequiresReconcile {
			t.Fatalf("action %q: unauthorized plan must not require reconcile", action)
		}
	}
}

// TestPersesLifecycleUnresolvedOwnershipFailClosed asserts the ownership guard:
// an unverified owner receives a fail-closed error even when authorized.
func TestPersesLifecycleUnresolvedOwnershipFailClosed(t *testing.T) {
	m := persesTestManifest(false)
	_, err := ResolvePersesLifecycle(m, PersesLifecycleEnable, true, true, false)
	if err == nil {
		t.Fatal("unresolved ownership must fail closed")
	}
}

// TestPersesLifecycleEnableSetsPersesFlag asserts enable sets services.perses
// and requires reconcile; disable clears it.
func TestPersesLifecycleEnableSetsPersesFlag(t *testing.T) {
	m := persesTestManifest(false)
	plan, err := ResolvePersesLifecycle(m, PersesLifecycleEnable, true, true, true)
	if err != nil {
		t.Fatalf("enable must succeed for authorized owner: %v", err)
	}
	if !plan.RequiresReconcile {
		t.Fatal("enable on an unselected manifest must require reconcile")
	}
	if !plan.ManifestAfter.Services.Perses {
		t.Fatal("enable plan must set services.perses=true")
	}
	if !plan.BackendPreserved {
		t.Fatal("enable plan must preserve backend data owners")
	}

	disablePlan, err := ResolvePersesLifecycle(plan.ManifestAfter, PersesLifecycleDisable, false, true, true)
	if err != nil {
		t.Fatalf("disable must succeed: %v", err)
	}
	if !disablePlan.RequiresReconcile {
		t.Fatal("disable on a selected manifest must require reconcile")
	}
	if disablePlan.ManifestAfter.Services.Perses {
		t.Fatal("disable plan must clear services.perses")
	}
}

// TestPersesLifecycleEnableIdempotentWhenAlreadySelected asserts that enabling
// an already-selected surface requires no reconcile.
func TestPersesLifecycleEnableIdempotentWhenAlreadySelected(t *testing.T) {
	m := persesTestManifest(true)
	plan, err := ResolvePersesLifecycle(m, PersesLifecycleEnable, true, true, true)
	if err != nil {
		t.Fatalf("enable must succeed: %v", err)
	}
	if plan.RequiresReconcile {
		t.Fatal("enable on an already-selected surface must not require reconcile")
	}
}

// TestPersesLifecycleRemovePreservesBackendDataOwners asserts that a remove plan
// disables the surface, removes only owned resources, and never touches backend
// data owners (Prometheus, Loki, Tempo).
func TestPersesLifecycleRemovePreservesBackendDataOwners(t *testing.T) {
	m := persesTestManifest(true)
	plan, err := ResolvePersesLifecycle(m, PersesLifecycleRemove, false, true, true)
	if err != nil {
		t.Fatalf("remove must succeed for authorized owner: %v", err)
	}
	if !plan.BackendPreserved {
		t.Fatal("remove must preserve backend data owners")
	}
	if !plan.OwnedResourcesRemoved {
		t.Fatal("remove plan must retire owned Perses resources")
	}
	if plan.ManifestAfter.Services.Perses {
		t.Fatal("remove must clear services.perses")
	}
	if !plan.RequiresReconcile {
		t.Fatal("remove must require reconcile to converge runtime")
	}
}

// TestPersesLifecycleRejectsPersesWithoutObservabilitySurface asserts the
// fail-closed invariant that services.perses without the observability
// management-ui surface is rejected (matching the manifest parser).
func TestPersesLifecycleRejectsPersesWithoutObservabilitySurface(t *testing.T) {
	m := Manifest{Version: 1, Name: "perses-app", Environment: "dev"}
	m.Services.Perses = true
	_, err := ResolvePersesLifecycle(m, PersesLifecycleDisable, false, true, true)
	if err == nil {
		t.Fatal("perses without observability management-ui surface must fail closed")
	}
}

// TestPersesLifecycleStatusThreeStates asserts the public three-state derivation
// is not_configured when unselected and configured_unverified when selected and
// pinned (0428-186). "verified" is a real per-target health-probe result and
// Core ships no such probe, so a selected+pinned surface with a silent/not-probed
// backend must be configured_unverified, never verified (CODEX-REVIEW fail-closed
// correction: verified is no longer derived from the static reference catalog).
func TestPersesLifecycleStatusThreeStates(t *testing.T) {
	if got := PersesLifecycleStatus(persesTestManifest(false)); got != "not_configured" {
		t.Fatalf("unselected perses status = %q, want not_configured", got)
	}
	if got := PersesLifecycleStatus(persesTestManifest(true)); got != "configured_unverified" {
		t.Fatalf("selected+pinned perses status without a real probe = %q, want configured_unverified", got)
	}
}

// TestPersesLifecycleBackendPreservedInvariant asserts that no plan this adapter
// produces can ever remove a backend data owner. This guards the core
// acceptance requirement: runtime stop/remove must not delete Prometheus, Loki,
// Tempo or foreign data.
func TestPersesLifecycleBackendPreservedInvariant(t *testing.T) {
	for _, action := range []PersesLifecycleAction{PersesLifecycleEnable, PersesLifecycleDisable, PersesLifecycleRemove} {
		for _, selected := range []bool{false, true} {
			m := persesTestManifest(selected)
			plan, err := ResolvePersesLifecycle(m, action, action == PersesLifecycleRemove || action == PersesLifecycleEnable, true, true)
			if err != nil {
				t.Fatalf("action %q selected=%v: unexpected error %v", action, selected, err)
			}
			if !plan.BackendPreserved {
				t.Fatalf("action %q selected=%v: plan must preserve backend data owners", action, selected)
			}
		}
	}
}
