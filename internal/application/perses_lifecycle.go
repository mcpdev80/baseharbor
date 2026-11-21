package application

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses lifecycle adapter.
//
// This adapter implements the accepted CLI/domain operation for the optional
// Perses observability management surface introduced by the v0.4.28 Perses
// provider seam. It is bounded to the already-present manifest contract and the
// existing ownership-safe runtime removal path; it never invents a new runtime
// provider and never touches Prometheus, Loki, Tempo or any other backend data.
//
// The three operations are:
//
//   - Enable: select the optional Perses realization of the observability
//     management-ui surface by setting services.perses (guarded by the same
//     fail-closed invariants the manifest parser enforces).
//   - Disable: deselect services.perses, returning the surface to its
//     not_configured three-state while leaving the backend observability
//     surface (Prometheus) in place.
//   - Remove: ownership-safe retirement of the owned Perses realization only.
//     It disables the surface first and removes only the BaseHarbor-owned
//     Perses runtime resources; backend data owners (Prometheus, Loki, Tempo)
//     are preserved and never removed.
//
// False readiness is forbidden: none of these operations asserts a running
// Perses health path. Enable reports "configured_unverified" (the only
// reachable state this Core produces for a selected surface), never "verified".

// PersesLifecycleAction is the operation a plan carries.
type PersesLifecycleAction string

const (
	// PersesLifecycleEnable selects the optional Perses management surface.
	PersesLifecycleEnable PersesLifecycleAction = "enable"
	// PersesLifecycleDisable deselects the optional Perses management surface.
	PersesLifecycleDisable PersesLifecycleAction = "disable"
	// PersesLifecycleRemove retires the owned Perses realization safely.
	PersesLifecycleRemove PersesLifecycleAction = "remove"
)

// PersesLifecycleError is a fail-closed lifecycle error. Every error returned by
// this adapter is a *PersesLifecycleError so callers can distinguish an
// authorization/ownership guard from an unrelated runtime failure.
type PersesLifecycleError struct {
	Reason string
	Detail string
}

func (e *PersesLifecycleError) Error() string {
	if strings.TrimSpace(e.Detail) != "" {
		return fmt.Sprintf("perses: %s: %s", e.Reason, e.Detail)
	}
	return "perses: " + e.Reason
}

func persesLifecycleError(reason, detail string) error {
	return &PersesLifecycleError{Reason: reason, Detail: detail}
}

// PersesLifecyclePlan is the outcome of resolving one lifecycle operation
// against manifest truth. It is a pure, runtime-independent contract: it never
// reaches for a running container or health probe, so it can be asserted
// deterministically.
type PersesLifecyclePlan struct {
	// Action is the operation this plan performs.
	Action PersesLifecycleAction
	// ManifestAfter is the manifest the operation converges to. For enable it
	// has services.perses set; for disable/remove it has it cleared. The caller
	// writes ManifestAfter back to the repository manifest and reconciles.
	ManifestAfter Manifest
	// RequiresReconcile reports that the repository manifest was changed and the
	// existing 'baha app apply' reconciliation must run to converge runtime.
	RequiresReconcile bool
	// BackendPreserved reports that the plan never removes a backend data owner
	// (Prometheus, Loki, Tempo); it is always true for every plan this adapter
	// produces and is asserted by tests to prevent regressions.
	BackendPreserved bool
	// OwnedResourcesRemoved counts the BaseHarbor-owned Perses runtime resources
	// a remove plan would retire. It is zero for enable/disable.
	OwnedResourcesRemoved bool
}

// ValidatePersesLifecycleIdentity enforces the ownership/authorization guard
// that every lifecycle operation carries before any manifest mutation. It mirrors
// the ownership check the CLI performs via authorizeApplicationOperation but is
// exported so the plan can be asserted fail-closed in isolation:
//
//   - The surface must be the optional observability management-ui realization,
//     never a new application capability.
//   - Enabling requires the observability management-ui surface to be selected
//     and managed metrics sources present (Prometheus is the surface); the same
//     fail-closed invariant the manifest parser enforces.
//
// It is fail-closed: an unknown or unauthorized caller receives an error and no
// plan.
func ValidatePersesLifecycleIdentity(m Manifest, authorized bool, ownershipVerified bool) error {
	if !authorized {
		return persesLifecycleError("not authorized", "perses lifecycle requires a verified application owner; access denied for unauthorized caller")
	}
	if !ownershipVerified {
		return persesLifecycleError("unresolved ownership", "perses lifecycle requires a verified application owner before any manifest mutation")
	}
	if m.Services.Perses && !m.Services.ObservabilityManagementUI {
		return persesLifecycleError("invalid surface selection", "services.perses requires the observability management-ui surface to be selected")
	}
	return nil
}

// ResolvePersesLifecycle computes the plan for a single lifecycle operation.
//
// setPerses is the desired resulting value of services.perses for the operation.
// For enable it is true, for disable/remove it is false. The function is fail-
// closed: it validates identity first, then the manifest invariant, then returns
// the runtime-independent plan.
func ResolvePersesLifecycle(m Manifest, action PersesLifecycleAction, setPerses bool, authorized bool, ownershipVerified bool) (PersesLifecyclePlan, error) {
	if err := ValidatePersesLifecycleIdentity(m, authorized, ownershipVerified); err != nil {
		return PersesLifecyclePlan{}, err
	}

	plan := PersesLifecyclePlan{
		Action:           action,
		BackendPreserved: true,
	}

	after := m
	if after.Services.Perses != setPerses {
		after.Services.Perses = setPerses
		plan.RequiresReconcile = true
	} else {
		plan.RequiresReconcile = false
	}

	if err := after.Validate(); err != nil {
		return PersesLifecyclePlan{}, persesLifecycleError("invalid manifest after lifecycle change", err.Error())
	}

	plan.ManifestAfter = after

	switch action {
	case PersesLifecycleRemove:
		// A remove plan retires the owned Perses realization only. Backend data
		// owners (Prometheus, Loki, Tempo) are preserved by construction.
		plan.OwnedResourcesRemoved = true
	case PersesLifecycleEnable, PersesLifecycleDisable:
		// Enable/disable only toggle the optional surface selection; they never
		// touch backend data owners.
	}

	return plan, nil
}

// PersesLifecycleStatus derives the public three-state contract for the optional
// Perses surface from manifest truth, reusing the seam's three-state mapper.
// It returns the exact state string (not_configured |
// configured_unverified) and never "verified": this Core ships no Perses health
// path, so a selected surface is always configured_unverified.
func PersesLifecycleStatus(m Manifest) string {
	status := capability.PersesStatusNow(m.Services.ObservabilityManagementUI && m.Services.Perses)
	if status == nil {
		return string(capability.PersesStatusNotConfigured)
	}
	return string(*status)
}

// PersesLifecycleEnabled reports whether the optional Perses surface is currently
// selected in the manifest.
func PersesLifecycleEnabled(m Manifest) bool {
	return m.Services.Perses
}

// PersesLifecycleRequiresObservabilitySurface reports the fail-closed precondition
// that enabling Perses requires the observability management-ui surface and
// managed metrics sources (Prometheus). This mirrors validateObservabilityUIPre
// ference so enable never produces an invalid manifest.
func PersesLifecycleRequiresObservabilitySurface(m Manifest) bool {
	return m.Services.ObservabilityManagementUI && HasMetricsSources(m)
}
