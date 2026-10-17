package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func appEditResultTestPlan(impacts ...application.ProviderImpact) application.ProviderImpactPlan {
	return application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, Target: "prod", Revision: "r-1", PlanID: "plan-1",
		Complete: true, ConfirmationsRequired: 1, Impacts: impacts,
	}
}

func measuredZeroFact(target string) application.ProviderFact {
	var zero int64
	return application.ProviderFact{Target: target, Containers: application.Quantity{Value: &zero, Classification: "measured"}}
}

func stoppedZeroFact(target string) application.ProviderFact {
	var zero int64
	return application.ProviderFact{Target: target, State: application.ProviderStoppedOwned, Containers: application.Quantity{Value: &zero, Classification: "measured"}}
}

func presentFact(target string) application.ProviderFact {
	one := int64(1)
	return application.ProviderFact{Target: target, State: application.ProviderRunning, Containers: application.Quantity{Value: &one, Classification: "measured"}}
}

func appEditResultResolved(target string) resolvedApplication {
	return resolvedApplication{Target: deployment.ResolvedTarget{Name: target}, Store: application.Store{Root: filepath.Join(os.TempDir(), "baha-nonexistent-store")}}
}

// appEditResultTestRegistry is the captured registry snapshot the projector reads
// for these direct-projection tests (it mirrors what the lifecycle records under
// the Target lock in production).
var appEditResultTestRegistry capability.Registry

func appEditResultEvidence(before, after map[string]application.ProviderFact) applicationApplyLifecycleEvidence {
	return applicationApplyLifecycleEvidence{Resolved: appEditResultResolved("prod"), Registry: appEditResultTestRegistry, Before: before, After: after}
}

// attestRegistry records BaseHarbor-owned application-scoped instances for the
// given ids, so realization attestation reads identity/ownership from the
// captured registry rather than from any proposal field.
func attestRegistry(t *testing.T, ownedIDs ...string) {
	t.Helper()
	registry := capability.Registry{}
	for _, id := range ownedIDs {
		registry.Instances = append(registry.Instances, capability.ProviderInstance{
			ID: id, Provider: capability.Provider{Kind: capability.ProviderValkey},
			Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: "app-1",
		})
	}
	prev := appEditResultTestRegistry
	t.Cleanup(func() { appEditResultTestRegistry = prev })
	appEditResultTestRegistry = registry
}

// denyRegistry records no owned instances, so add/stop/replace receipts stay
// pending while retirements still attest.
func denyRegistry(t *testing.T) {
	t.Helper()
	prev := appEditResultTestRegistry
	t.Cleanup(func() { appEditResultTestRegistry = prev })
	appEditResultTestRegistry = capability.Registry{}
}

func TestAppApplyObservedOutcomesMarksOnlyRetiredAbsence(t *testing.T) {
	denyRegistry(t)
	plan := appEditResultTestPlan(
		application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"},
		application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderRunning}},
	)
	before := map[string]application.ProviderFact{"sql-old": presentFact("prod")}
	after := map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod"), "cache": presentFact("prod")}

	got := appApplyObservedOutcomes(plan, appEditResultEvidence(before, after))
	// Only the retirement is attestation-eligible; the add stays pending because
	// no authoritative registry fact confirms the new realization.
	want := []application.ProviderExecutionOutcome{
		{InstanceID: "sql-old", Operation: "destroy", Status: "applied", Verified: true, Evidence: "observed owned provider runtime physically retired by this Apply lifecycle run"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observed outcomes = %+v, want %+v", got, want)
	}
}

func TestAppApplyObservedOutcomesNeverFabricatesApplied(t *testing.T) {
	cases := map[string]struct {
		impact application.ProviderImpact
		before map[string]application.ProviderFact
		after  map[string]application.ProviderFact
	}{
		"still running after": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"},
			map[string]application.ProviderFact{"sql-old": presentFact("prod")},
			map[string]application.ProviderFact{"sql-old": presentFact("prod")},
		},
		"absent and never present before": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"},
			map[string]application.ProviderFact{},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
		},
		"measured zero before cannot become a retirement receipt": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
		},
		"removal with retained material is pending": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy", Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "sql-old/vol", Ownership: capability.OwnershipBaseHarbor}}}},
			map[string]application.ProviderFact{"sql-old": presentFact("prod")},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
		},
		"shared unbind is not presence-attestable": {
			application.ProviderImpact{InstanceID: "shared-cache", Operation: "unbind"},
			map[string]application.ProviderFact{"shared-cache": presentFact("prod")},
			map[string]application.ProviderFact{"shared-cache": presentFact("prod")},
		},
		"add without any change is pending": {
			application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderRunning}},
			map[string]application.ProviderFact{"cache": presentFact("prod")},
			map[string]application.ProviderFact{"cache": presentFact("prod")},
		},
		"add carrying material consequences stays pending": {
			application.ProviderImpact{InstanceID: "cache", Operation: "add",
				After:     &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderRunning},
				Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "cache/vol", Ownership: capability.OwnershipBaseHarbor}}}},
			map[string]application.ProviderFact{},
			map[string]application.ProviderFact{"cache": presentFact("prod")},
		},
		"stop without real running predecessor is pending": {
			application.ProviderImpact{InstanceID: "cache", Operation: "stop", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderStoppedOwned}},
			map[string]application.ProviderFact{"cache": stoppedZeroFact("prod")},
			map[string]application.ProviderFact{"cache": stoppedZeroFact("prod")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan := appEditResultTestPlan(tc.impact)
			for _, out := range appApplyObservedOutcomes(plan, appEditResultEvidence(tc.before, tc.after)) {
				if out.Status == "applied" {
					t.Fatalf("fabricated applied receipt: %+v", out)
				}
			}
		})
	}
}

func TestAppApplyObservedOutcomesStopAndReplaceTransitions(t *testing.T) {
	attestRegistry(t, "cache", "sql-new")
	stopPlan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "stop",
		After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderStoppedOwned}})
	stop := appApplyObservedOutcomes(stopPlan, appEditResultEvidence(
		map[string]application.ProviderFact{"cache": presentFact("prod")},
		map[string]application.ProviderFact{"cache": stoppedZeroFact("prod")}))
	if len(stop) != 1 || stop[0].Status != "applied" || stop[0].After == nil || stop[0].After.State != application.ProviderStoppedOwned || !stop[0].After.Verified {
		t.Fatalf("stop transition not attested: %+v", stop)
	}

	replacePlan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "replace",
		After: &application.ProviderRealization{InstanceID: "sql-new", Target: "prod", Kind: "postgres", State: application.ProviderRunning}})
	replace := appApplyObservedOutcomes(replacePlan, appEditResultEvidence(
		map[string]application.ProviderFact{"sql-old": presentFact("prod"), "sql-new": measuredZeroFact("prod")},
		map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod"), "sql-new": presentFact("prod")}))
	if len(replace) != 1 || replace[0].Status != "applied" || replace[0].After.InstanceID != "sql-new" {
		t.Fatalf("provider replacement not attested: %+v", replace)
	}

	// Replacement where the old runtime survived is NOT this run's doing.
	bad := appApplyObservedOutcomes(replacePlan, appEditResultEvidence(
		map[string]application.ProviderFact{"sql-old": presentFact("prod")},
		map[string]application.ProviderFact{"sql-old": presentFact("prod"), "sql-new": presentFact("prod")}))
	for _, out := range bad {
		if out.Status == "applied" {
			t.Fatalf("replacement attested although old runtime survived: %+v", out)
		}
	}
}

func TestAppApplyObservedOutcomesRequiresEvidence(t *testing.T) {
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	if got := appApplyObservedOutcomes(plan, applicationApplyLifecycleEvidence{}); got != nil {
		t.Fatalf("outcomes from absent evidence must stay pending: %+v", got)
	}
}

func swapApplyResultSeams(t *testing.T, resolve func(context.Context, application.Store, []string, string) (resolvedApplication, error), observe func(context.Context, resolvedApplication) map[string]application.ProviderFact, run func(context.Context, application.Store, []string, io.Writer, io.Writer, applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error)) {
	t.Helper()
	prevResolve, prevObserve, prevRun := resolveApplicationFunc, observeApplicationProviderFactsFunc, applicationApplyLifecycleRunWithHooks
	t.Cleanup(func() {
		resolveApplicationFunc, observeApplicationProviderFactsFunc, applicationApplyLifecycleRunWithHooks = prevResolve, prevObserve, prevRun
	})
	if resolve != nil {
		resolveApplicationFunc = resolve
	}
	if observe != nil {
		observeApplicationProviderFactsFunc = observe
	}
	if run != nil {
		applicationApplyLifecycleRunWithHooks = run
	}
}

func noopObserve(context.Context, resolvedApplication) map[string]application.ProviderFact {
	return map[string]application.ProviderFact{}
}

// lifecycleSeam mimics the concrete hook contract: the mutate callback runs
// inside the (simulated) Target lock and a failing mutation prevents any
// convergence, exactly like runApplicationApplyLifecycle.
func lifecycleSeam(calls *int, converged *int, runErr error, evidence applicationApplyLifecycleEvidence) func(context.Context, application.Store, []string, io.Writer, io.Writer, applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
	return func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, hooks applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
		*calls++
		if hooks.Mutate != nil {
			if err := hooks.Mutate(ctx, appEditResultResolved("prod")); err != nil {
				return applicationApplyLifecycleEvidence{}, err
			}
		}
		*converged++
		return evidence, runErr
	}
}

func TestExecuteApplicationApplyLifecycleResultsDrivesLifecycleOnceAndPreservesError(t *testing.T) {
	calls, converged := 0, 0
	runErr := errors.New("provider verification failed after retirement")
	evidence := appEditResultEvidence(
		map[string]application.ProviderFact{"sql-old": presentFact("prod")},
		map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")})
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("prod"), nil
		},
		noopObserve,
		lifecycleSeam(&calls, &converged, runErr, evidence),
	)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	outcomes, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, plan, nil, nil, nil)
	if calls != 1 || converged != 1 {
		t.Fatalf("lifecycle calls=%d converged=%d, want exactly 1 each", calls, converged)
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("error not preserved: %v", err)
	}
	// The genuinely observed earlier retirement is reported even though the
	// overall run failed afterwards.
	if len(outcomes) != 1 || outcomes[0].Status != "applied" {
		t.Fatalf("partial effect after later failure not attested: %+v", outcomes)
	}
}

func TestExecuteApplicationApplyLifecycleResultsMutateFailureConvergesNothing(t *testing.T) {
	calls, converged := 0, 0
	mutateErr := errors.New("mutate refused")
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("prod"), nil
		},
		noopObserve,
		lifecycleSeam(&calls, &converged, nil, applicationApplyLifecycleEvidence{}),
	)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderRunning}})
	outcomes, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, plan, func(context.Context, resolvedApplication) error {
		return mutateErr
	}, nil, nil)
	if !errors.Is(err, mutateErr) {
		t.Fatalf("mutate error not preserved: %v", err)
	}
	if converged != 0 || len(outcomes) != 0 {
		t.Fatalf("lifecycle converged after refused mutation: calls=%d outcomes=%+v", converged, outcomes)
	}
}

func TestExecuteApplicationApplyLifecycleResultsTargetMismatchRunsNothing(t *testing.T) {
	calls, converged := 0, 0
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("dev"), nil
		},
		noopObserve,
		lifecycleSeam(&calls, &converged, nil, applicationApplyLifecycleEvidence{}),
	)
	if _, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, appEditResultTestPlan(), nil, nil, nil); !errors.Is(err, errAppApplyResultTargetMismatch) {
		t.Fatalf("expected target mismatch, got %v", err)
	}
	if calls != 0 || converged != 0 {
		t.Fatalf("lifecycle ran on target mismatch: calls=%d", calls)
	}
}

func TestExecuteApplicationApplyLifecycleResultsRejectsInvalidPlan(t *testing.T) {
	calls := 0
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			calls++
			return resolvedApplication{}, nil
		},
		noopObserve, nil,
	)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "prod", PlanID: ""}
	if _, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, plan, nil, nil, nil); err == nil {
		t.Fatal("expected invalid plan rejection")
	}
	if calls != 0 {
		t.Fatal("resolved before plan validation")
	}
}

// TestObserveApplyLifecycleAfterSurvivesCancellation locks finding 4: the
// post-run observation must not inherit the canceled mutation context.
func TestObserveApplyLifecycleAfterSurvivesCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	seen := map[string]any{}
	prev := observeApplicationProviderFactsFunc
	t.Cleanup(func() { observeApplicationProviderFactsFunc = prev })
	observeApplicationProviderFactsFunc = func(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
		seen["err"] = ctx.Err()
		_, bounded := ctx.Deadline()
		seen["bounded"] = bounded
		return map[string]application.ProviderFact{"cache": presentFact("prod")}
	}
	facts := observeApplyLifecycleAfter(parent, appEditResultResolved("prod"))
	if err, _ := seen["err"].(error); err != nil {
		t.Fatalf("post-observation inherited the canceled context: %v", err)
	}
	if bounded, _ := seen["bounded"].(bool); !bounded {
		t.Fatal("post-observation must run under a bounded independent context")
	}
	if facts["cache"].State != application.ProviderRunning {
		t.Fatalf("partial effects lost after cancellation: %+v", facts)
	}
}

func TestAppEditRuntimeExecuteAuthorityRunsLifecycleAndSourceOnce(t *testing.T) {
	sourceWrites, lifecycleRuns, mutateCalls := 0, 0, 0
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	desired := appEditDesired{Target: "prod", ApplicationID: "app-1", Manifest: application.New("edit-safe", "dev", false, false, false), Requirements: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), []byte("original"),
		appEditTestAuthorities("prod", "app-1", desired.Requirements, nil, []application.ProviderImpactPlan{plan}, new(int)),
		nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(ctx context.Context, _ application.Store, _ []string, _ application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, _, _ io.Writer) ([]application.ProviderExecutionOutcome, error) {
			lifecycleRuns++
			if mutate == nil {
				t.Fatal("Edit executor must pass an in-lock mutation step")
			}
			mutateCalls++
			if err := mutate(ctx, appEditResultResolved("prod")); err != nil {
				return nil, err
			}
			return []application.ProviderExecutionOutcome{{InstanceID: "sql-old", Operation: "destroy", Status: "applied", Verified: true, Evidence: "retired"}}, nil
		},
	)
	outcomes, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true})
	if err != nil || mutateCalls != 1 || sourceWrites != 1 || lifecycleRuns != 1 {
		t.Fatalf("mutate=%d writes=%d runs=%d err=%v", mutateCalls, sourceWrites, lifecycleRuns, err)
	}
	if len(outcomes) != 1 || outcomes[0].Status != "applied" {
		t.Fatalf("outcomes not propagated: %+v", outcomes)
	}
}

func TestAppEditRuntimeExecuteAuthorityForeignSourceAbortsBeforeConvergence(t *testing.T) {
	lifecycleRuns, converged := 0, 0
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	desired := appEditDesired{Target: "prod", ApplicationID: "app-1", Manifest: application.New("edit-safe", "dev", false, false, false)}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), []byte("original"),
		appEditTestAuthorities("prod", "app-1", nil, nil, []application.ProviderImpactPlan{plan}, new(int)),
		nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error {
			return errAppEditSourceForeignEdit
		},
		func(ctx context.Context, _ application.Store, _ []string, _ application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, _, _ io.Writer) ([]application.ProviderExecutionOutcome, error) {
			lifecycleRuns++
			if err := mutate(ctx, appEditResultResolved("prod")); err != nil {
				return nil, err
			}
			converged++
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}); !errors.Is(err, errAppEditSourceForeignEdit) {
		t.Fatalf("expected foreign source abort, got %v", err)
	}
	if converged != 0 {
		t.Fatalf("lifecycle converged after source write failed: %d", converged)
	}
	if lifecycleRuns != 1 {
		t.Fatalf("lifecycle entry must be reached exactly once (lock acquisition), runs=%d", lifecycleRuns)
	}
}

func TestAppEditRuntimeExecuteAuthorityPlanDriftWritesNothing(t *testing.T) {
	approved := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey", State: application.ProviderRunning}})
	drifted := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "postgres", State: application.ProviderRunning}})
	sourceWrites := 0
	desired := appEditDesired{Target: "prod", ApplicationID: "app-1", Manifest: application.New("edit-safe", "dev", false, false, false), Requirements: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), nil,
		appEditTestAuthorities("prod", "app-1", desired.Requirements, nil, []application.ProviderImpactPlan{drifted}, new(int)),
		nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(ctx context.Context, _ application.Store, _ []string, _ application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, _, _ io.Writer) ([]application.ProviderExecutionOutcome, error) {
			if err := mutate(ctx, appEditResultResolved("prod")); err != nil {
				return nil, err
			}
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, approved, application.ProviderChangeConsent{PlanID: approved.PlanID, First: true}); !errors.Is(err, errAppEditPlanDrifted) {
		t.Fatalf("expected plan drift abort, got %v", err)
	}
	if sourceWrites != 0 {
		t.Fatal("source written after plan drift")
	}
}

func TestAppEditRuntimeExecuteAuthorityIncompleteAuthoritiesWriteNothing(t *testing.T) {
	calls := 0
	desired := appEditDesired{Target: "prod"}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, nil, appEditResultResolved("prod"), nil, appEditAuthorities{},
		nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { calls++; return nil },
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, func(context.Context, resolvedApplication) error, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
			calls++
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, appEditResultTestPlan(), application.ProviderChangeConsent{}); err == nil {
		t.Fatal("expected authority completeness failure")
	}
	if calls != 0 {
		t.Fatal("incomplete authorities reached a mutation path")
	}
}

func TestAppEditRuntimeExecuteAuthorityTargetMismatchWritesNothing(t *testing.T) {
	sourceWrites := 0
	plan := appEditResultTestPlan()
	desired := appEditDesired{Target: "dev"}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), nil,
		appEditTestAuthorities("prod", "", nil, nil, []application.ProviderImpactPlan{plan}, new(int)),
		nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, func(context.Context, resolvedApplication) error, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{}); !errors.Is(err, errAppApplyResultTargetMismatch) {
		t.Fatalf("expected target mismatch, got %v", err)
	}
	if sourceWrites != 0 {
		t.Fatal("wrote source on target mismatch")
	}
}

func TestDefaultAppEditLifecycleRunnerBindsConcreteResultsPath(t *testing.T) {
	calls, converged := 0, 0
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("prod"), nil
		},
		noopObserve,
		lifecycleSeam(&calls, &converged, nil, applicationApplyLifecycleEvidence{Before: map[string]application.ProviderFact{}, After: map[string]application.ProviderFact{}}),
	)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	outcomes, err := defaultAppEditLifecycleRunner(context.Background(), application.Store{}, []string{"edit-safe"}, plan, nil, nil, nil)
	if err != nil || calls != 1 {
		t.Fatalf("concrete binding calls=%d err=%v", calls, err)
	}
	if outcomes == nil {
		t.Fatal("nil outcomes from concrete connection")
	}
}

func TestDefaultAppEditSourceWriterRepositoryCAS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseharbor.yaml")
	manifest := application.WithSQLInstances(application.New("edit-safe", "dev", false, false, false), "primary")
	original := []byte("# keep this comment\n" + manifest.AuthoredYAML())
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := appEditResultResolved("dev")
	resolved.FromRepository = true
	resolved.ManifestPath = path

	editted := application.WithSQLInstances(application.WithCacheInstances(application.New("edit-safe", "dev", false, true, false), "cache"), "primary")
	if err := defaultAppEditSourceWriter([]byte("stale-original"))(context.Background(), resolved, editted); !errors.Is(err, errAppEditSourceForeignEdit) {
		t.Fatalf("expected foreign edit abort, got %v", err)
	}
	if err := defaultAppEditSourceWriter(original)(context.Background(), resolved, editted); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	round, err := application.ParseYAML(string(written))
	if err != nil {
		t.Fatal(err)
	}
	if !round.Services.SQL || !round.Services.Cache {
		t.Fatalf("edited intent not persisted: %+v", round.Services)
	}
	if !bytes.Contains(written, []byte("edit-safe")) {
		t.Fatal("application identity lost")
	}
}

// TestAtomicWriteManifestRejectsRacingEdit locks the finding 2 CAS rule: a
// change occurring after the initial compare must still abort the replace.
func TestAtomicWriteManifestRejectsRacingEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseharbor.yaml")
	original := []byte("original-bytes\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	// A racing foreign change after the caller's compare: the recheck inside
	// the write must observe it and abort.
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = os.WriteFile(path, []byte("foreign-change\n"), 0o600)
	}()
	start := time.Now()
	var err error
	for time.Since(start) < 2*time.Second {
		err = atomicWriteApplicationManifest(path, original, []byte("edited"))
		if errors.Is(err, errAppEditSourceForeignEdit) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !errors.Is(err, errAppEditSourceForeignEdit) {
		t.Fatalf("racing edit was clobbered instead of aborted: %v", err)
	}
}

// TestAppEditRuntimeExecuteAuthorityPersistsOnlyAfterInLockRevalidation proves
// the ordering requirement end to end on the executor surface: revalidation
// output precedes the source write within a single lifecycle entry.
func TestAppEditRuntimeExecuteAuthorityPersistsOnlyAfterInLockRevalidation(t *testing.T) {
	var order []string
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	desired := appEditDesired{Target: "prod", ApplicationID: "app-1", Manifest: application.New("edit-safe", "dev", false, false, false), Requirements: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}}
	authorities := appEditTestAuthorities("prod", "app-1", desired.Requirements, nil, []application.ProviderImpactPlan{plan}, new(int))
	read := authorities.Read
	authorities.Read = func(ctx context.Context, target string) (GuidedReadContext, error) {
		order = append(order, "read")
		return read(ctx, target)
	}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), []byte("original"), authorities, nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error {
			order = append(order, "write")
			return nil
		},
		func(ctx context.Context, _ application.Store, _ []string, _ application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, _, _ io.Writer) ([]application.ProviderExecutionOutcome, error) {
			order = append(order, "lock")
			if err := mutate(ctx, appEditResultResolved("prod")); err != nil {
				return nil, err
			}
			order = append(order, "converge")
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(order, ",")
	if !strings.HasPrefix(joined, "lock,read") || !strings.Contains(joined, "write,converge") {
		t.Fatalf("mutation ordering wrong: %s", joined)
	}
	for _, item := range order {
		if item == "write" && strings.Index(joined, "lock") > strings.Index(joined, "write") {
			t.Fatal("source written outside the lifecycle lock")
		}
	}
}

// TestDefaultAppEditAuthoritiesAreCallable proves the delivered handler
// assembles production authorities (no injected fixtures) for registration by
// core-921.
func TestDefaultAppEditAuthoritiesAreCallable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	authorities := defaultAppEditAuthorities(application.Store{}, nil)
	if authorities.Read == nil || authorities.Choices == nil || authorities.Plan == nil || authorities.Preview == nil || authorities.Authorize == nil {
		t.Fatalf("default authorities incomplete: %+v", authorities)
	}
	if err := authorities.Authorize(context.Background(), "missing", "foreign"); err == nil {
		t.Fatal("authorize accepted an unresolvable target")
	}
	if _, err := authorities.Read(context.Background(), "missing"); err == nil {
		t.Fatal("production read succeeded without any application")
	}
}

// fixtureTargetName resolves a real Target through the accepted configuration.
func fixtureTargetName(t *testing.T) string {
	t.Helper()
	return configureTestTarget(t).Name
}

// writeWorkloadFixture creates an http workload app that passes the pre-lock
// admission checks, so the lifecycle reaches its bracketed observation and then
// stops at runtime detection without requiring a container.
func writeWorkloadFixture(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  demo-app:\n    build: .\n    ports:\n      - '8080:8080'\n")
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"demo-app"}}}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
}

// TestReview918RealLifecycleReturnsAfterFacts proves finding 2 on the ACTUAL
// hook-bearing lifecycle: runApplicationApplyLifecycle (no lifecycleSeam)
// populates evidence.After through its named-return defer even when a later step
// fails, so genuine partial effects survive.
func TestReview918RealLifecycleReturnsAfterFacts(t *testing.T) {
	name := fixtureTargetName(t)
	writeWorkloadFixture(t)

	prev := observeApplicationProviderFactsFunc
	t.Cleanup(func() { observeApplicationProviderFactsFunc = prev })
	calls := 0
	observeApplicationProviderFactsFunc = func(context.Context, resolvedApplication) map[string]application.ProviderFact {
		calls++
		if calls == 1 {
			return map[string]application.ProviderFact{"sql-primary": presentFact(name)}
		}
		return map[string]application.ProviderFact{"sql-primary": measuredZeroFact(name)}
	}

	evidence, _ := runApplicationApplyLifecycle(context.Background(), application.DefaultStore(), nil, io.Discard, io.Discard, applicationApplyLifecycleHooks{CollectEvidence: true})
	if evidence.Resolved.Target.Name != name || evidence.Before == nil || evidence.After == nil {
		t.Fatalf("real lifecycle did not populate bracketed observations: resolved=%q before=%v after=%v", evidence.Resolved.Target.Name, evidence.Before, evidence.After)
	}
	if calls < 2 {
		t.Fatalf("post-observation did not run (observe calls=%d)", calls)
	}

	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: name, Revision: "r", PlanID: "p", Complete: true, ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{{InstanceID: "sql-primary", Operation: "destroy"}}}
	outcomes := appApplyObservedOutcomes(plan, evidence)
	if len(outcomes) != 1 || outcomes[0].Status != "applied" {
		t.Fatalf("real-lifecycle retirement not attested: %+v", outcomes)
	}
}

// TestReview918RetirementRepreparedAfterEdit proves finding 3: after the in-lock
// source mutation, runApplicationApplyLifecycle re-prepares retirement for the
// rebuilt resolved state instead of reusing a stale or zero providerPlan.
func TestReview918RetirementRepreparedAfterEdit(t *testing.T) {
	fixtureTargetName(t)
	writeWorkloadFixture(t)

	prev := prepareApplicationProviderRetirementFunc
	t.Cleanup(func() { prepareApplicationProviderRetirementFunc = prev })
	prepared := 0
	prepareApplicationProviderRetirementFunc = func(ctx context.Context, resolved resolvedApplication) (application.ProviderPlan, error) {
		prepared++
		return prev(ctx, resolved)
	}
	mutateCalled := 0
	_, _ = runApplicationApplyLifecycle(context.Background(), application.DefaultStore(), nil, io.Discard, io.Discard, applicationApplyLifecycleHooks{
		Mutate: func(context.Context, resolvedApplication) error { mutateCalled++; return nil },
	})
	if mutateCalled != 1 {
		t.Fatalf("mutate calls = %d, want 1", mutateCalled)
	}
	if prepared < 2 {
		t.Fatalf("retirement was not re-prepared after the source mutation: prepared=%d", prepared)
	}
}

// TestReview918AlreadyAbsentCannotBecomeApplied proves a measured-zero BEFORE is
// not treated as runtime presence, so a no-change retirement is not applied.
func TestReview918AlreadyAbsentCannotBecomeApplied(t *testing.T) {
	denyRegistry(t)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	evidence := appEditResultEvidence(map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")}, map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")})
	for _, out := range appApplyObservedOutcomes(plan, evidence) {
		if out.Status == "applied" {
			t.Fatalf("already-absent instance attested as retired: %+v", out)
		}
	}
}

// TestReview918RuntimeCountsCannotVerifyProposedOwner proves finding 4: an Owner
// supplied only by the proposal is not verified by runtime counts; the realized
// owner comes from the registry, so the domain recorder must not report complete.
func TestReview918RuntimeCountsCannotVerifyProposedOwner(t *testing.T) {
	attestRegistry(t, "cache")
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{
		InstanceID: "cache", Target: "prod", Kind: "valkey", Scope: capability.ScopeApplication,
		Ownership: capability.OwnershipBaseHarbor, Owner: "owner-only-in-proposal", State: application.ProviderRunning,
	}})
	evidence := appEditResultEvidence(
		map[string]application.ProviderFact{"cache": measuredZeroFact("prod")},
		map[string]application.ProviderFact{"cache": presentFact("prod")})
	outcomes := appApplyObservedOutcomes(plan, evidence)
	result, err := application.RecordProviderChangeResult(plan, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == "complete" {
		t.Fatalf("proposal-only owner verified from runtime counts: %+v", result)
	}
	for _, out := range outcomes {
		if out.After != nil && out.After.Owner == "owner-only-in-proposal" {
			t.Fatalf("proposal owner copied into receipt: %+v", out.After)
		}
	}
}

// TestReview918EditExecutorDefaultBindsRealLifecycle proves the Edit executor's
// default (non-injected) connection reaches the real hook-bearing
// runApplicationApplyLifecycle, not a seam that returns preconstructed evidence.
func TestReview918EditExecutorDefaultBindsRealLifecycle(t *testing.T) {
	name := fixtureTargetName(t)
	writeWorkloadFixture(t)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "edit")
	if err != nil {
		t.Fatal(err)
	}
	prevRun, prevObserve := applicationApplyLifecycleRunWithHooks, observeApplicationProviderFactsFunc
	t.Cleanup(func() {
		applicationApplyLifecycleRunWithHooks, observeApplicationProviderFactsFunc = prevRun, prevObserve
	})
	calls, realRuns := 0, 0
	observeApplicationProviderFactsFunc = func(context.Context, resolvedApplication) map[string]application.ProviderFact {
		calls++
		if calls == 1 {
			return map[string]application.ProviderFact{"sql-primary": presentFact(name)}
		}
		return map[string]application.ProviderFact{"sql-primary": measuredZeroFact(name)}
	}
	applicationApplyLifecycleRunWithHooks = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, hooks applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
		realRuns++
		return runApplicationApplyLifecycle(ctx, store, args, out, errOut, hooks)
	}
	desired := appEditDesired{Target: name, ApplicationID: resolved.Manifest.ApplicationID, Manifest: resolved.Manifest, Requirements: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}}
	executor := appEditRuntimeExecuteAuthority(application.DefaultStore(), nil, resolved, nil, defaultAppEditAuthorities(application.DefaultStore(), nil), io.Discard, io.Discard,
		func(context.Context, resolvedApplication, application.Manifest) error { return nil }, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: name, Revision: "r", PlanID: "p", Complete: true, ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{{InstanceID: "sql-primary", Operation: "destroy"}}}
	_, _ = executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true})
	if realRuns != 1 {
		t.Fatalf("real lifecycle runs = %d, want exactly 1 via the default connection", realRuns)
	}
}
