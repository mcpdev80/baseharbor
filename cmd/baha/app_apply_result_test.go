package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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

func presentFact(target string) application.ProviderFact {
	one := int64(1)
	return application.ProviderFact{Target: target, State: application.ProviderRunning, Containers: application.Quantity{Value: &one, Classification: "measured"}}
}

func appEditResultResolved(target string) resolvedApplication {
	return resolvedApplication{Target: deployment.ResolvedTarget{Name: target}, Store: application.Store{Root: filepath.Join(os.TempDir(), "baha-nonexistent-store")}}
}

func TestAppApplyObservedOutcomesMarksOnlyRetiredAbsence(t *testing.T) {
	plan := appEditResultTestPlan(
		application.ProviderImpact{InstanceID: "sql-old", Operation: "replace"},
		application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey"}},
	)
	before := map[string]application.ProviderFact{"sql-old": presentFact("prod")}
	after := map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod"), "cache": presentFact("prod")}

	got := appApplyObservedOutcomes("prod", plan, before, after)
	want := []application.ProviderExecutionOutcome{{InstanceID: "sql-old", Operation: "replace", Status: "applied", Verified: true, Evidence: "observed owned provider runtime physically retired by this Apply lifecycle run"}}
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
		"absent but not present before": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"},
			map[string]application.ProviderFact{},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
		},
		"removal with retained material is pending": {
			application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy", Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "sql-old/vol", Ownership: capability.OwnershipBaseHarbor}}}},
			map[string]application.ProviderFact{"sql-old": presentFact("prod")},
			map[string]application.ProviderFact{"sql-old": measuredZeroFact("prod")},
		},
		"add with after realization is pending": {
			application.ProviderImpact{InstanceID: "cache", Operation: "add", After: &application.ProviderRealization{InstanceID: "cache", Target: "prod", Kind: "valkey"}},
			map[string]application.ProviderFact{},
			map[string]application.ProviderFact{"cache": presentFact("prod")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan := appEditResultTestPlan(tc.impact)
			for _, out := range appApplyObservedOutcomes("prod", plan, tc.before, tc.after) {
				if out.Status == "applied" {
					t.Fatalf("fabricated applied receipt: %+v", out)
				}
			}
		})
	}
}

func swapApplyResultSeams(t *testing.T, resolve func(context.Context, application.Store, []string, string) (resolvedApplication, error), observe func(context.Context, resolvedApplication) map[string]application.ProviderFact, run func(context.Context, application.Store, []string, io.Writer, io.Writer) error) {
	t.Helper()
	prevResolve, prevObserve, prevRun := resolveApplicationFunc, observeApplicationProviderFactsFunc, applicationApplyLifecycleRun
	t.Cleanup(func() {
		resolveApplicationFunc, observeApplicationProviderFactsFunc, applicationApplyLifecycleRun = prevResolve, prevObserve, prevRun
	})
	if resolve != nil {
		resolveApplicationFunc = resolve
	}
	if observe != nil {
		observeApplicationProviderFactsFunc = observe
	}
	if run != nil {
		applicationApplyLifecycleRun = run
	}
}

func noopObserve(context.Context, resolvedApplication) map[string]application.ProviderFact {
	return map[string]application.ProviderFact{}
}

func TestExecuteApplicationApplyLifecycleResultsDrivesLifecycleOnceAndPreservesError(t *testing.T) {
	calls := 0
	runErr := errors.New("provider verification failed after retirement")
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("prod"), nil
		},
		noopObserve,
		func(context.Context, application.Store, []string, io.Writer, io.Writer) error { calls++; return runErr },
	)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	outcomes, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, plan, nil, nil)
	if calls != 1 {
		t.Fatalf("lifecycle calls = %d, want exactly 1", calls)
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("error not preserved: %v", err)
	}
	for _, o := range outcomes {
		if o.Status == "applied" {
			t.Fatalf("no genuine retirement observed, yet outcome applied: %+v", o)
		}
	}
}

func TestExecuteApplicationApplyLifecycleResultsTargetMismatchWritesNothing(t *testing.T) {
	calls := 0
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("dev"), nil
		},
		noopObserve,
		func(context.Context, application.Store, []string, io.Writer, io.Writer) error { calls++; return nil },
	)
	if _, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, appEditResultTestPlan(), nil, nil); !errors.Is(err, errAppApplyResultTargetMismatch) {
		t.Fatalf("expected target mismatch, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("lifecycle ran on target mismatch: %d", calls)
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
	if _, err := executeApplicationApplyLifecycleResults(context.Background(), application.Store{}, nil, plan, nil, nil); err == nil {
		t.Fatal("expected invalid plan rejection")
	}
	if calls != 0 {
		t.Fatal("resolved before plan validation")
	}
}

func TestAppEditRuntimeExecuteAuthorityRunsLifecycleAndSourceOnce(t *testing.T) {
	sourceWrites, lifecycleRuns := 0, 0
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	desired := appEditDesired{Target: "prod", Manifest: application.New("edit-safe", "dev", false, false, false), Requirements: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), []byte("original"), nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
			lifecycleRuns++
			return []application.ProviderExecutionOutcome{{InstanceID: "sql-old", Operation: "destroy", Status: "applied", Verified: true, Evidence: "retired"}}, nil
		},
	)
	outcomes, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true})
	if err != nil || sourceWrites != 1 || lifecycleRuns != 1 {
		t.Fatalf("writes=%d runs=%d err=%v", sourceWrites, lifecycleRuns, err)
	}
	if len(outcomes) != 1 || outcomes[0].Status != "applied" {
		t.Fatalf("outcomes not propagated: %+v", outcomes)
	}
}

func TestAppEditRuntimeExecuteAuthorityForeignSourceAbortsBeforeLifecycle(t *testing.T) {
	lifecycleRuns := 0
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	desired := appEditDesired{Target: "prod", Manifest: application.New("edit-safe", "dev", false, false, false)}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), []byte("original"), nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error {
			return errAppEditSourceForeignEdit
		},
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
			lifecycleRuns++
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}); !errors.Is(err, errAppEditSourceForeignEdit) {
		t.Fatalf("expected foreign source abort, got %v", err)
	}
	if lifecycleRuns != 0 {
		t.Fatalf("lifecycle ran after source write failed: %d", lifecycleRuns)
	}
}

func TestAppEditRuntimeExecuteAuthorityConsentMismatchWritesNothing(t *testing.T) {
	sourceWrites, lifecycleRuns := 0, 0
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "cache", Operation: "destroy"})
	desired := appEditDesired{Target: "prod"}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), nil, nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
			lifecycleRuns++
			return nil, nil
		},
	)
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID}); err == nil {
		t.Fatal("expected consent validation failure")
	}
	if sourceWrites != 0 || lifecycleRuns != 0 {
		t.Fatalf("mutation before consent: writes=%d runs=%d", sourceWrites, lifecycleRuns)
	}
}

func TestAppEditRuntimeExecuteAuthorityTargetMismatchWritesNothing(t *testing.T) {
	sourceWrites := 0
	plan := appEditResultTestPlan()
	desired := appEditDesired{Target: "dev"}
	executor := appEditRuntimeExecuteAuthority(
		application.Store{}, []string{"edit-safe"}, appEditResultResolved("prod"), nil, nil, nil,
		func(context.Context, resolvedApplication, application.Manifest) error { sourceWrites++; return nil },
		func(context.Context, application.Store, []string, application.ProviderImpactPlan, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error) {
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
	calls := 0
	swapApplyResultSeams(t,
		func(context.Context, application.Store, []string, string) (resolvedApplication, error) {
			return appEditResultResolved("prod"), nil
		},
		noopObserve,
		func(context.Context, application.Store, []string, io.Writer, io.Writer) error { calls++; return nil },
	)
	plan := appEditResultTestPlan(application.ProviderImpact{InstanceID: "sql-old", Operation: "destroy"})
	outcomes, err := defaultAppEditLifecycleRunner(context.Background(), application.Store{}, []string{"edit-safe"}, plan, nil, nil)
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
