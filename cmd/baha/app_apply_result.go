package main

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appApplyEvidenceObservationTimeout bounds the independent read-only
// post-observation window. A run that was canceled or failed must still yield
// whatever its Target state really is; the bound keeps a wedged runtime from
// hanging the receipt path forever.
const appApplyEvidenceObservationTimeout = 20 * time.Second

// errAppApplyResultTargetMismatch prevents binding observed receipts to a plan
// for a different Target than the resolved application.
var errAppApplyResultTargetMismatch = errors.New("apply result plan Target differs from the resolved application")

// errAppEditSourceForeignEdit aborts source persistence when the repository file
// changed after hydration, so a concurrent or unrelated edit is never clobbered.
var errAppEditSourceForeignEdit = errors.New("application source changed since it was loaded")

// resolveApplicationFunc is the accepted read-only application resolver used to
// scope the result-bearing run to the resolved Target. It defaults to
// resolveApplication.
var resolveApplicationFunc = resolveApplication

// observeApplicationProviderFactsFunc is the accepted read-only provider
// observer used by the evidence-collecting lifecycle run. It defaults to
// observeApplicationProviderFacts.
var observeApplicationProviderFactsFunc = observeApplicationProviderFacts

// applicationApplyLifecycleRunWithHooks is the single, concrete Apply lifecycle
// boundary the result-bearing path drives. It defaults to
// runApplicationApplyLifecycle so production receipts come from one real
// lifecycle run under the existing Target lock; tests may substitute it only to
// observe that the concrete connection is driven exactly once.
var applicationApplyLifecycleRunWithHooks = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, hooks applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
	return runApplicationApplyLifecycle(ctx, store, args, out, errOut, hooks)
}

// executeApplicationApplyLifecycleResults is the result-bearing Apply entry
// point introduced for core-918. It drives the existing lifecycle exactly once
// through the hook-bearing run: the caller's mutate step executes inside the
// Target lifecycle lock immediately before the first state write, and the
// authoritative before/after provider observations are bracketed inside the same
// lock around that single run. All six error-only callers and their
// lock/authorization/consent/preflight/ownership/verify behavior are unchanged.
//
// Only genuinely observed native effects are returned as receipts. Every other
// approved impact is intentionally omitted so application.RecordProviderChangeResult
// reports it as pending. A plan is required so a receipt is never derived from a
// nil aggregate error; a failing mutate step prevents the lifecycle from
// converging anything and preserves the original error.
func executeApplicationApplyLifecycleResults(ctx context.Context, store application.Store, args []string, plan application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, out, errOut io.Writer) ([]application.ProviderExecutionOutcome, error) {
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
	evidence, runErr := applicationApplyLifecycleRunWithHooks(ctx, store, args, out, errOut, applicationApplyLifecycleHooks{
		Mutate:          mutate,
		CollectEvidence: true,
	})
	return appApplyObservedOutcomes(plan, evidence), runErr
}

// observeApplyLifecycleAfter performs the post-run read-only observation on an
// independent, bounded context derived from the run WITHOUT its cancellation.
// A canceled or failed mutation must still yield usable partial evidence; an
// observation failure leaves the respective map incomplete, which the
// projector reports as pending/unknown and never as success.
func observeApplyLifecycleAfter(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appApplyEvidenceObservationTimeout)
	defer cancel()
	return observeApplicationProviderFactsFunc(observeCtx, resolved)
}

// appApplyObservedOutcomes projects ACTUAL native observations bound to this
// invocation onto the approved plan. It never copies the plan's proposed After,
// material, effect or function values into a receipt as a success claim, and it
// never marks an unobserved impact applied. An impact that cannot be attested
// by the bracketed observations is omitted, so the domain recorder reports it
// pending and the overall result partial.
func appApplyObservedOutcomes(plan application.ProviderImpactPlan, evidence applicationApplyLifecycleEvidence) []application.ProviderExecutionOutcome {
	if evidence.Before == nil || evidence.After == nil {
		return nil
	}
	target := plan.Target
	outcomes := make([]application.ProviderExecutionOutcome, 0, len(plan.Impacts))
	for _, impact := range plan.Impacts {
		if appImpactIsAttestedRetirement(impact) {
			if appFactMeasuredPresent(target, evidence.Before[impact.InstanceID]) &&
				appFactMeasuredAbsent(target, evidence.After[impact.InstanceID]) {
				outcomes = append(outcomes, application.ProviderExecutionOutcome{
					InstanceID: impact.InstanceID,
					Operation:  impact.Operation,
					Status:     "applied",
					Verified:   true,
					Evidence:   "observed owned provider runtime physically retired by this Apply lifecycle run",
				})
			}
			continue
		}
		if outcome, ok := appAttestedRealizationOutcome(target, impact, evidence); ok {
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}

// appImpactIsAttestedRetirement reports whether an approved impact is a pure
// physical retirement the Target observation can attest without material,
// function, access or After evidence. Shared binding releases (unbind) are NOT
// retirement attestable: the shared runtime keeps serving other consumers, so
// presence proves nothing about this invocation.
func appImpactIsAttestedRetirement(impact application.ProviderImpact) bool {
	switch impact.Operation {
	case "destroy", "remove-runtime", "retire":
	default:
		return false
	}
	if impact.After != nil {
		return false
	}
	return len(impact.Materials) == 0 && len(impact.Effects) == 0 && len(impact.Functions) == 0 &&
		len(impact.ContinuationDependencies) == 0
}

// appAttestedRealizationOutcome attests state transitions that the bracketed
// native observations genuinely prove: a runtime realized by this invocation
// (add/resume/start), an owned runtime stopped by it, or a verified provider
// replacement. Impacts carrying material, function, effect, access or location
// evidence stay omitted: this observation cannot attest those and a receipt must
// never copy the proposal to pass the domain recorder.
func appAttestedRealizationOutcome(target string, impact application.ProviderImpact, evidence applicationApplyLifecycleEvidence) (application.ProviderExecutionOutcome, bool) {
	if impact.After == nil || impact.After.Target != target || impact.After.Access != nil || len(impact.After.Locations) != 0 {
		return application.ProviderExecutionOutcome{}, false
	}
	if len(impact.Materials) != 0 || len(impact.Functions) != 0 || len(impact.Effects) != 0 || len(impact.ContinuationDependencies) != 0 {
		return application.ProviderExecutionOutcome{}, false
	}
	realized := impact.Operation == "add" || impact.Operation == "resume" || impact.Operation == "start"
	stopping := impact.Operation == "stop"
	replacing := impact.Operation == "replace"
	if !realized && !stopping && !replacing {
		return application.ProviderExecutionOutcome{}, false
	}
	after := evidence.After[impact.After.InstanceID]
	before := evidence.Before[impact.After.InstanceID]
	switch {
	case stopping:
		// Stopped owned runtime: measured zero presence in the expected state,
		// and a real running predecessor, so the stop belongs to this run.
		if !appFactMeasuredStopped(target, after) || !appFactMeasuredRunning(target, before) {
			return application.ProviderExecutionOutcome{}, false
		}
	case realized, replacing:
		if impact.After.State != application.ProviderRunning || !appFactMeasuredRunning(target, after) {
			return application.ProviderExecutionOutcome{}, false
		}
		// No-ops never produce a realization receipt.
		if appFactMeasuredRunning(target, before) {
			return application.ProviderExecutionOutcome{}, false
		}
		if replacing {
			// The replaced runtime must have been retired by this same run.
			old, oldAfter := evidence.After[impact.InstanceID]
			if !appFactMeasuredPresent(target, evidence.Before[impact.InstanceID]) || !oldAfter || !appFactMeasuredAbsent(target, old) {
				return application.ProviderExecutionOutcome{}, false
			}
		}
	}
	outcome := application.ProviderExecutionOutcome{
		InstanceID: impact.InstanceID,
		Operation:  impact.Operation,
		Status:     "applied",
		Verified:   true,
		Materials:  []application.MaterialOutcome{},
		Evidence:   "observed provider runtime state verified by this Apply lifecycle run",
	}
	observed := *impact.After
	observed.State = after.State
	observed.Verified = true
	observed.Evidence = "observed authoritative Target provider observation during this Apply lifecycle run"
	outcome.After = &observed
	return outcome, true
}

// appFactMeasuredPresent requires an actual measured runtime with at least one
// owned container on the expected Target; a measured-zero fact is absence, not
// presence, even when its instance key exists in the map.
func appFactMeasuredPresent(target string, fact application.ProviderFact) bool {
	return fact.Target == target &&
		fact.Containers.Classification == "measured" &&
		fact.Containers.Value != nil &&
		*fact.Containers.Value > 0
}

func appFactMeasuredRunning(target string, fact application.ProviderFact) bool {
	return fact.State == application.ProviderRunning && appFactMeasuredPresent(target, fact)
}

func appFactMeasuredStopped(target string, fact application.ProviderFact) bool {
	return fact.Target == target &&
		fact.State == application.ProviderStoppedOwned &&
		fact.Containers.Classification == "measured" &&
		fact.Containers.Value != nil &&
		*fact.Containers.Value == 0
}

// appFactMeasuredAbsent mirrors the authoritative absence test used by
// verifyApplicationRuntimeRetirement: measured zero containers on the Target.
func appFactMeasuredAbsent(target string, fact application.ProviderFact) bool {
	return fact.Target == target &&
		fact.Containers.Value != nil &&
		fact.Containers.Classification == "measured" &&
		*fact.Containers.Value == 0
}
