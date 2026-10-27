package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appApplyEvidenceObservationTimeout bounds the independent read-only
// post-observation window so a wedged runtime cannot hang the receipt path.
const appApplyEvidenceObservationTimeout = 20 * time.Second

// errAppApplyResultTargetMismatch prevents binding observed receipts to a plan
// for a different Target than the resolved application.
var errAppApplyResultTargetMismatch = errors.New("apply result plan Target differs from the resolved application")

// errAppEditSourceForeignEdit aborts source persistence when the repository file
// changed after hydration, so a concurrent edit is never clobbered.
var errAppEditSourceForeignEdit = errors.New("application source changed since it was loaded")

// resolveApplicationFunc scopes the result-bearing run to the resolved Target.
// It defaults to resolveApplication.
var resolveApplicationFunc = resolveApplication

// observeApplicationProviderFactsFunc is the read-only provider observer used by
// the evidence-collecting run. It defaults to observeApplicationProviderFacts.
var observeApplicationProviderFactsFunc = observeApplicationProviderFacts

// prepareApplicationProviderRetirementFunc prepares retirement/authorization.
// The lifecycle calls it for the pre-edit and, under the same lock, the
// post-edit resolved state. It defaults to prepareApplicationProviderRetirement.
var prepareApplicationProviderRetirementFunc = prepareApplicationProviderRetirement

// applicationApplyLifecycleRunWithHooks is the concrete lifecycle boundary the
// result-bearing path drives; it defaults to runApplicationApplyLifecycle.
var applicationApplyLifecycleRunWithHooks = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, hooks applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
	return runApplicationApplyLifecycle(ctx, store, args, out, errOut, hooks)
}

// executeApplicationApplyLifecycleResults drives the lifecycle once through the
// hook-bearing run: the mutate step executes inside the Target lock before the
// first state write, and before/after observations are bracketed in the same lock.
// A plan is required so a receipt is never derived from a nil aggregate error; a
// failing mutate preserves the original error and converges nothing. Unobserved
// impacts are omitted so the domain recorder reports them pending.
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
	hooks := applicationApplyLifecycleHooks{
		Mutate:          mutate,
		CollectEvidence: true,
	}
	if providerChangeConsentFromContext(ctx) != nil {
		approved := plan
		hooks.ApprovedPlan = &approved
	}
	evidence, runErr := applicationApplyLifecycleRunWithHooks(ctx, store, args, out, errOut, hooks)
	return appApplyObservedOutcomes(plan, evidence), runErr
}

// authorizeConfirmedApplicationEdit reuses an Edit acknowledgement only when
// the post-mutation plan differs solely in plan/revision identity or evidence
// text. Any changed consequence, verification state or confirmation requirement
// remains stale and requires fresh consent.
func authorizeConfirmedApplicationEdit(ctx context.Context, resolved resolvedApplication, approved application.ProviderImpactPlan, in io.Reader, out io.Writer) error {
	consent := providerChangeConsentFromContext(ctx)
	if consent == nil {
		return authorizeApplicationProviderChanges(ctx, resolved, in, out)
	}
	if err := application.ValidateProviderChangeConsent(approved, *consent); err != nil {
		return classifyProviderChangeConsent(err)
	}
	current, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		return err
	}
	if current.ProviderPlan.Impact == nil || !sameAppEditAuthorizationSurface(approved, *current.ProviderPlan.Impact) {
		return classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	forwarded := *consent
	forwarded.PlanID = current.ProviderPlan.Impact.PlanID
	return authorizeApplicationProviderChanges(withProviderChangeConsent(ctx, &forwarded), resolved, in, out)
}

func sameAppEditAuthorizationSurface(left, right application.ProviderImpactPlan) bool {
	l, err := appEditAuthorizationSurface(left)
	if err != nil {
		return false
	}
	r, err := appEditAuthorizationSurface(right)
	return err == nil && bytes.Equal(l, r)
}

func appEditAuthorizationSurface(plan application.ProviderImpactPlan) ([]byte, error) {
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	stripAppEditAuthorizationMetadata(value, true)
	return json.Marshal(value)
}

func stripAppEditAuthorizationMetadata(value any, top bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "evidence" || top && (key == "plan_id" || key == "revision") {
				delete(typed, key)
				continue
			}
			stripAppEditAuthorizationMetadata(child, false)
		}
	case []any:
		for _, child := range typed {
			stripAppEditAuthorizationMetadata(child, false)
		}
	}
}

// observeApplyLifecycleAfter does the post-run read-only observation on a
// bounded context derived from the run WITHOUT its cancellation, so a canceled
// or failed run still yields usable partial evidence.
func observeApplyLifecycleAfter(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appApplyEvidenceObservationTimeout)
	defer cancel()
	return observeApplicationProviderFactsFunc(observeCtx, resolved)
}

// appApplyObservedOutcomes binds actual observations to the approved plan. It
// never copies the plan's proposed After or carries evidence into a success
// claim, and never marks an unobserved impact applied; unattestable impacts are
// omitted so the domain recorder reports them pending.
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
		} else if outcome, ok := appAttestedSQLNewConsumer(target, impact, evidence); ok {
			outcomes = append(outcomes, outcome)
		} else if outcome, ok := appObservedPendingUpdate(target, impact, evidence); ok {
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}

// An update with unresolved consumer/material consequences can still return
// its measured runtime observation. It remains pending, never applied.
func appObservedPendingUpdate(target string, impact application.ProviderImpact, evidence applicationApplyLifecycleEvidence) (application.ProviderExecutionOutcome, bool) {
	if impact.Operation != "update" || impact.After == nil {
		return application.ProviderExecutionOutcome{}, false
	}
	before, hadBefore := evidence.Before[impact.InstanceID]
	after, hadAfter := evidence.After[impact.InstanceID]
	if !hadBefore || !hadAfter || before.Containers.Classification != "measured" || !appFactMeasuredRunning(target, after) {
		return application.ProviderExecutionOutcome{}, false
	}
	observed, ok := appAuthoritativeRealization(evidence.Registry, target, impact.InstanceID, after)
	if !ok {
		return application.ProviderExecutionOutcome{}, false
	}
	observed.Verified = true
	observed.Evidence = "measured running provider runtime and authoritative Target registry; consumer and material outcomes not attested"
	return application.ProviderExecutionOutcome{
		InstanceID: impact.InstanceID,
		Operation:  impact.Operation,
		Status:     "pending",
		Materials:  []application.MaterialOutcome{},
		Evidence:   "runtime state observed after Apply; SQL consumer and data consequences remain unverified",
		After:      &observed,
	}, true
}

// Attest only a new application-owned SQL realization with real native
// container transition, canonical registry ownership and a credential-bound
// consumer query captured after this same lifecycle run. Existing SQL updates,
// non-empty inventories and migrations cannot enter this success path.
func appAttestedSQLNewConsumer(target string, impact application.ProviderImpact, evidence applicationApplyLifecycleEvidence) (application.ProviderExecutionOutcome, bool) {
	if (impact.Operation != "add" && impact.Operation != "resume") || impact.After == nil || impact.After.Kind != capability.ProviderPostgreSQL ||
		impact.After.Scope != capability.ScopeApplication || len(impact.Materials) != 0 ||
		len(impact.Effects) != 0 || len(impact.ContinuationDependencies) != 0 ||
		len(impact.Functions) == 0 || impact.After.Access != nil || len(impact.After.Locations) != 0 {
		return application.ProviderExecutionOutcome{}, false
	}
	before, beforeKnown := evidence.Before[impact.InstanceID]
	after, afterKnown := evidence.After[impact.InstanceID]
	if !beforeKnown || !afterKnown || !appFactMeasuredAbsent(target, before) || !appFactMeasuredRunning(target, after) {
		return application.ProviderExecutionOutcome{}, false
	}
	actual, ok := appAuthoritativeRealization(evidence.Registry, target, impact.InstanceID, after)
	inventory, measured := evidence.SQL[impact.InstanceID]
	if !ok || !measured || !inventory.Empty ||
		inventory.Consumer == "" || inventory.Instance == "" || inventory.Evidence == "" ||
		actual.Owner == "" || actual.Owner != "target/"+target && actual.Scope != capability.ScopeApplication {
		return application.ProviderExecutionOutcome{}, false
	}
	matchesOwner := false
	for _, instance := range evidence.Registry.Instances {
		if instance.ID == impact.InstanceID && instance.OwnerApplicationID != "" &&
			inventory.Consumer == "application/"+instance.OwnerApplicationID+"/"+instance.OwnerEnvironment &&
			instance.Scope == capability.ScopeApplication && instance.Ownership == capability.OwnershipBaseHarbor {
			matchesOwner = true
		}
	}
	if !matchesOwner {
		return application.ProviderExecutionOutcome{}, false
	}
	functions := make([]application.ProviderFunctionOutcome, 0, len(impact.Functions))
	for _, need := range impact.Functions {
		// A newly reachable consumer SELECT is measured; runtime/catalog
		// checks cannot attest handoff, expiry or custom behavior.
		if need.Consumer != inventory.Consumer || need.Function != string(capability.SQL) ||
			len(need.Via) != 1 || need.Via[0] != impact.InstanceID ||
			need.ValidUntil != "" || need.Behavior != "" || need.Transition != nil {
			return application.ProviderExecutionOutcome{}, false
		}
		functions = append(functions, application.ProviderFunctionOutcome{
			Consumer:   inventory.Consumer,
			InstanceID: impact.InstanceID,
			Function:   string(capability.SQL),
			Outcome:    "changed",
			Verified:   true,
			Evidence:   inventory.Evidence,
		})
	}
	actual.Verified = true
	actual.Evidence = "owned SQL container changed from measured absent to running; canonical Target registry and native application consumer query attest realization"
	return application.ProviderExecutionOutcome{
		InstanceID: impact.InstanceID,
		Operation:  impact.Operation,
		Status:     "applied",
		Verified:   true,
		Evidence:   "same-lock runtime transition, Target-owned provider and actual scoped SQL consumer query",
		After:      &actual,
		Functions:  functions,
		Materials:  []application.MaterialOutcome{},
	}, true
}

// appImpactIsAttestedRetirement reports a pure physical retirement the Target
// observation can attest without material, function, access or After evidence.
// Shared unbind is excluded: the shared runtime keeps serving other consumers,
// so presence proves nothing about this invocation.
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

// appProviderRegistryFunc loads the authoritative Target provider registry. The
// evidence-collecting lifecycle calls it once, inside the Target lock, so identity
// and ownership are captured with the runtime facts rather than re-read later.
var appProviderRegistryFunc = func(resolved resolvedApplication) (capability.Registry, error) {
	return (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
}

// appAuthoritativeRealization derives a realization from the captured registry
// and the observed fact, never from the proposal.
func appAuthoritativeRealization(registry capability.Registry, target string, instanceID string, fact application.ProviderFact) (application.ProviderRealization, bool) {
	if fact.Target != "" && fact.Target != target {
		return application.ProviderRealization{}, false
	}
	for _, instance := range registry.Instances {
		if instance.ID != instanceID || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		owner := "target/" + target
		if instance.Scope == capability.ScopeApplication {
			owner = instance.OwnerApplicationID
		}
		if fact.Ancillary {
			owner = fact.OwnerReference
		}
		return application.ProviderRealization{
			InstanceID: instanceID,
			Kind:       instance.Provider.Kind,
			Target:     target,
			Scope:      instance.Scope,
			Ownership:  instance.Ownership,
			Owner:      owner,
			Ancillary:  fact.Ancillary,
			State:      fact.State,
		}, true
	}
	return application.ProviderRealization{}, false
}

// appAttestedRealizationOutcome attests add/resume/start, stop and replace
// transitions the observations and registry genuinely prove. Realized identity
// and ownership come from the registry, never copied from the proposal; impacts
// with material, function, effect, access or location evidence stay pending.
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
	afterFact, afterKnown := evidence.After[impact.After.InstanceID]
	beforeFact, beforeKnown := evidence.Before[impact.After.InstanceID]
	if !afterKnown || !beforeKnown || beforeFact.Containers.Classification != "measured" {
		// Reject add/start/stop when the BEFORE inventory is unknown; a missing
		// or unmeasured fact is not proven absence and cannot support a claim.
		return application.ProviderExecutionOutcome{}, false
	}
	observed, attested := appAuthoritativeRealization(evidence.Registry, target, impact.After.InstanceID, afterFact)
	if !attested {
		// Identity/ownership could not be attested from the authoritative
		// registry; report pending rather than copy the proposal.
		return application.ProviderExecutionOutcome{}, false
	}
	switch {
	case stopping:
		if !appFactMeasuredStopped(target, afterFact) || !appFactMeasuredRunning(target, beforeFact) {
			return application.ProviderExecutionOutcome{}, false
		}
	case realized, replacing:
		if !appFactMeasuredRunning(target, afterFact) {
			return application.ProviderExecutionOutcome{}, false
		}
		// No-ops never produce a realization receipt.
		if appFactMeasuredRunning(target, beforeFact) {
			return application.ProviderExecutionOutcome{}, false
		}
		if replacing {
			old, oldAfter := evidence.After[impact.InstanceID]
			if !appFactMeasuredPresent(target, evidence.Before[impact.InstanceID]) || !oldAfter || !appFactMeasuredAbsent(target, old) {
				return application.ProviderExecutionOutcome{}, false
			}
		}
	}
	observed.Verified = true
	observed.Evidence = "attested from authoritative Target registry and runtime observation during this Apply lifecycle run"
	return application.ProviderExecutionOutcome{
		InstanceID: impact.InstanceID,
		Operation:  impact.Operation,
		Status:     "applied",
		Verified:   true,
		Materials:  []application.MaterialOutcome{},
		Evidence:   "observed provider runtime state verified by this Apply lifecycle run",
		After:      &observed,
	}, true
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
