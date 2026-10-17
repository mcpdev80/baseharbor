package main

import (
	"context"
	"errors"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// errAppApplyResultTargetMismatch prevents binding observed receipts to a plan
// for a different Target than the resolved application.
var errAppApplyResultTargetMismatch = errors.New("apply result plan Target differs from the resolved application")

// errAppEditSourceForeignEdit aborts source persistence when the repository file
// changed after hydration, so a concurrent or unrelated edit is never clobbered.
var errAppEditSourceForeignEdit = errors.New("application source changed since it was loaded")

// applicationApplyLifecycleRun is the single, concrete Apply lifecycle boundary
// the result-bearing path drives. It defaults to executeApplicationApplyLifecycle
// so production receipts come from a real lifecycle run, never an injected stub.
// Tests may substitute it only to prove the concrete connection is driven exactly
// once; the production binding is exercised by the Apply command and its native
// Docker/Podman gates.
var applicationApplyLifecycleRun = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	return executeApplicationApplyLifecycle(ctx, store, args, out, errOut)
}

// resolveApplicationFunc is the accepted read-only application resolver used to
// scope observations to the resolved Target. It defaults to resolveApplication.
var resolveApplicationFunc = resolveApplication

// observeApplicationProviderFactsFunc is the accepted read-only provider
// observer used to capture actual native effects around the lifecycle run. It
// defaults to observeApplicationProviderFacts.
var observeApplicationProviderFactsFunc = observeApplicationProviderFacts

// executeApplicationApplyLifecycleResults is the result-bearing Apply entry
// point introduced for core-918. It reuses the existing lifecycle exactly once
// through applicationApplyLifecycleRun (all error-only callers and their
// lock/authorization/consent/preflight/ownership/verify behavior are unchanged)
// and binds the actual runtime effects to the caller-supplied approved plan.
//
// Only genuinely observed native effects are returned as receipts. Every other
// approved impact is intentionally omitted so application.RecordProviderChangeResult
// reports it as pending. A plan is required so a receipt is never derived from a
// nil aggregate error; without a plan the wrapper stays error-only compatible.
func executeApplicationApplyLifecycleResults(ctx context.Context, store application.Store, args []string, plan application.ProviderImpactPlan, out, errOut io.Writer) ([]application.ProviderExecutionOutcome, error) {
	if plan.Version != application.ProviderImpactVersion || plan.PlanID == "" || plan.Target == "" {
		return nil, io.ErrUnexpectedEOF
	}
	resolved, err := resolveApplicationFunc(ctx, store, args, "apply")
	if err != nil {
		return nil, err
	}
	if resolved.Target.Name != plan.Target {
		return nil, errAppApplyResultTargetMismatch
	}
	// Observation is strictly read-only. It brackets the single authorized
	// lifecycle run so a real effect that occurred before a later failure is
	// still reported, and a nil error never implies a complete per-provider
	// receipt.
	before := observeApplicationProviderFactsFunc(ctx, resolved)
	runErr := applicationApplyLifecycleRun(ctx, store, args, out, errOut)
	after := observeApplicationProviderFactsFunc(ctx, resolved)
	return appApplyObservedOutcomes(resolved.Target.Name, plan, before, after), runErr
}

// appApplyObservedOutcomes projects ACTUAL native observations onto the approved
// plan. It never copies the plan's proposed After, material, effect or function
// values into a receipt, and it never marks an unobserved impact applied.
//
// A removal-style impact (the approved instance is expected to leave service
// while it retains no BaseHarbor-attested material, effect or function) is only
// reported applied when the Target observation proves the owned runtime was
// physically retired by this very invocation. Anything that cannot be fully
// observed through these hooks stays pending and is surfaced as partial by the
// domain recorder.
func appApplyObservedOutcomes(target string, plan application.ProviderImpactPlan, before, after map[string]application.ProviderFact) []application.ProviderExecutionOutcome {
	outcomes := make([]application.ProviderExecutionOutcome, 0, len(plan.Impacts))
	for _, impact := range plan.Impacts {
		if !appImpactIsObservedRetirement(impact) {
			continue
		}
		if !appProviderRuntimePhysicallyAbsent(target, after[impact.InstanceID]) {
			continue
		}
		// The instance must have been present for this invocation to have caused
		// its removal; otherwise absence is not a native effect of this run.
		if _, present := before[impact.InstanceID]; !present {
			continue
		}
		outcomes = append(outcomes, application.ProviderExecutionOutcome{
			InstanceID: impact.InstanceID,
			Operation:  impact.Operation,
			Status:     "applied",
			Verified:   true,
			Evidence:   "observed owned provider runtime physically retired by this Apply lifecycle run",
		})
	}
	return outcomes
}

// appImpactIsObservedRetirement reports whether an approved impact is a pure
// retirement that the Target observation can attest without additional
// material, function or After evidence.
func appImpactIsObservedRetirement(impact application.ProviderImpact) bool {
	switch impact.Operation {
	case "destroy", "unbind", "retire", "stop", "remove-runtime", "remove", "replace":
	default:
		return false
	}
	if impact.After != nil {
		return false
	}
	return len(impact.Materials) == 0 && len(impact.Effects) == 0 && len(impact.Functions) == 0
}

// appProviderRuntimePhysicallyAbsent mirrors the authoritative absence test used
// by verifyApplicationRuntimeRetirement: the owned provider runtime is measured
// at zero containers on the expected Target.
func appProviderRuntimePhysicallyAbsent(target string, fact application.ProviderFact) bool {
	return fact.Target == target &&
		fact.Containers.Value != nil &&
		fact.Containers.Classification == "measured" &&
		*fact.Containers.Value == 0
}
