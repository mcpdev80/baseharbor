package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func appWizardTestInput(t *testing.T) appWizardInput {
	t.Helper()
	m := application.New("catalog", "dev", false, false, false)
	m.ApplicationID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	m = application.WithSQLInstances(m, "primary", "analytics")
	reg := capability.NewRegistry()
	reg.Instances = []capability.ProviderInstance{
		{ID: "sql-a", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "a"},
		{ID: "sql-b", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "b"},
	}
	if err := reg.Validate(); err != nil {
		t.Fatal(err)
	}
	return appWizardInput{
		Manifest: m, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: reg},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{
			{Kind: capability.SQL, Name: "primary"}, {Kind: capability.SQL, Name: "analytics"},
		}}, TTY: true,
		Refresh: func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			fp := &GuidedReadContext{}
			fp.Footprint.Target = d.Target
			fp.Footprint.Complete = true
			impact := &application.ProviderImpactPlan{Target: d.Target, Complete: true, PlanID: "current", Version: application.ProviderImpactVersion}
			for _, req := range d.Requirements {
				impact.Impacts = append(impact.Impacts, application.ProviderImpact{InstanceID: req.Name, Operation: "retain"})
			}
			for _, pref := range d.Preferences {
				if pref.InstanceID != "" {
					impact.Impacts = append(impact.Impacts, application.ProviderImpact{InstanceID: pref.InstanceID, Operation: "select"})
				}
			}
			return fp, impact, nil
		},
	}
}

func appWizardAdvance(t *testing.T, w appWizardController, event appWizardEvent) appWizardController {
	t.Helper()
	n, err := w.Step(context.Background(), event)
	if err != nil {
		t.Fatalf("transition %s: %v", event, err)
	}
	return n
}

func TestAppWizardControllerRepeatedBackPreservesNamedResourcesAndProviders(t *testing.T) {
	input := appWizardTestInput(t)
	original := input.Manifest
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		for j, id := range []string{"sql-a", "sql-b"} {
			w, err = w.SelectProvider(context.Background(), input.Selection.Current[j], id)
			if err != nil {
				t.Fatal(err)
			}
		}
		w = appWizardAdvance(t, w, appWizardNext)
		if w.phase != appWizardPreview || !strings.Contains(w.View(), "sql-a") {
			t.Fatal(w.View())
		}
		w = appWizardAdvance(t, w, appWizardNo)
		if w.phase != appWizardNeeds || !reflect.DeepEqual(w.selection.Requirements(), input.Selection.Current) {
			t.Fatal("No did not restore checklist")
		}
		if w.preferences[appWizardKey("dev", input.Selection.Current[0])] != "sql-a" {
			t.Fatal("preference lost")
		}
	}
	w = appWizardAdvance(t, w, appWizardNext)
	w = appWizardAdvance(t, w, appWizardYes)
	d, ok := w.Decision()
	if !ok || len(d.Preferences) != 2 || d.Preferences[0].InstanceID != "sql-a" || d.Preferences[1].InstanceID != "sql-b" {
		t.Fatalf("decision: %+v", d)
	}
	if !reflect.DeepEqual(input.Manifest, original) {
		t.Fatal("original mutated")
	}
}

func TestAppWizardControllerChangedNeedsBackRetainsExistingComplexState(t *testing.T) {
	input := appWizardTestInput(t)
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	w = appWizardAdvance(t, w, appWizardNo)
	w, err = w.SelectNeed(context.Background(), "space")
	if err != nil || len(w.selection.Requirements()) != 0 {
		t.Fatal("deselection failed")
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if strings.Contains(w.View(), "primary: retain") {
		t.Fatal("stale preview")
	}
	w = appWizardAdvance(t, w, appWizardNo)
	w, err = w.SelectNeed(context.Background(), "space")
	if err != nil || !reflect.DeepEqual(w.selection.Requirements(), input.Selection.Current) {
		t.Fatal("names lost")
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if !strings.Contains(w.View(), "primary: retain") {
		t.Fatal("fresh consequences missing")
	}
}

func TestAppWizardControllerCancellationAtEveryTransitionLeavesFilesAndInputs(t *testing.T) {
	for _, stage := range []appWizardPhase{appWizardNeeds, appWizardPreview} {
		for _, event := range []appWizardEvent{appWizardCancel, appWizardEOF} {
			t.Run(string(stage)+"/"+string(event), func(t *testing.T) {
				input := appWizardTestInput(t)
				manifest := input.Manifest
				registry := input.Snapshot.Registry
				dir := t.TempDir()
				path := filepath.Join(dir, "manifest.yaml")
				data := []byte("name: preserved\\ncomplex: [one, two]\\nsecrets: unchanged\\n")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				w, err := newAppWizardController(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				if stage == appWizardPreview {
					w = appWizardAdvance(t, w, appWizardNext)
				}
				w, err = w.Step(context.Background(), event)
				if !errors.Is(err, errAppWizardAbort) || w.phase != appWizardAborted {
					t.Fatal(err)
				}
				if _, ok := w.Decision(); ok {
					t.Fatal("unexpected decision")
				}
				after, err := os.ReadFile(path)
				if err != nil || !reflect.DeepEqual(after, data) || !reflect.DeepEqual(manifest, input.Manifest) || !reflect.DeepEqual(registry, input.Snapshot.Registry) {
					t.Fatal("state or file mutated")
				}
			})
		}
	}
}

func TestAppWizardControllerContextAbortAtEveryTransition(t *testing.T) {
	for _, stage := range []appWizardPhase{appWizardNeeds, appWizardPreview} {
		w, err := newAppWizardController(context.Background(), appWizardTestInput(t))
		if err != nil {
			t.Fatal(err)
		}
		if stage == appWizardPreview {
			w = appWizardAdvance(t, w, appWizardNext)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		next, err := w.Step(ctx, appWizardYes)
		if !errors.Is(err, context.Canceled) || next.phase != appWizardAborted {
			t.Fatal(err)
		}
	}
}

func TestAppWizardControllerStaleAndWrongTargetPreview(t *testing.T) {
	for _, kind := range []string{"missing", "wrong-target", "stale"} {
		t.Run(kind, func(t *testing.T) {
			input := appWizardTestInput(t)
			calls := 0
			original := input.Refresh
			input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
				calls++
				if kind == "missing" {
					return nil, nil, nil
				}
				fp, impact, err := original(ctx, d)
				if kind == "wrong-target" {
					impact.Target = "prod"
				}
				if kind == "stale" && calls > 1 {
					impact.PlanID = "changed"
				}
				return fp, impact, err
			}
			w, err := newAppWizardController(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			next, err := w.Step(context.Background(), appWizardNext)
			if kind != "stale" {
				if err == nil || next.phase != appWizardNeeds {
					t.Fatal("accepted missing or wrong target")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = next.Step(context.Background(), appWizardYes); err == nil {
				t.Fatal("accepted stale preview")
			}
		})
	}
}

func TestAppWizardControllerObservationRefreshDoesNotInvalidatePreview(t *testing.T) {
	input := appWizardTestInput(t)
	calls := 0
	original := input.Refresh
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		calls++
		fp, impact, err := original(ctx, d)
		if err != nil {
			return nil, nil, err
		}
		fact := application.ProviderFact{Target: d.Target, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{}, MemoryBytes: application.Quantity{Classification: "unknown"}}
		if calls > 1 {
			n := int64(2048)
			fact.MemoryBytes = application.Quantity{Value: &n, Classification: "measured", Source: "runtime sample"}
		}
		countdown := int64(30 - calls)
		impact.Impacts = append(impact.Impacts, application.ProviderImpact{Materials: []application.MaterialConsequence{{Meaning: "within observed time window", Before: application.ProviderMaterial{ID: "certificate", ExpiresAt: "2036-11-13T05:39:05Z"}, RemainingValiditySeconds: &countdown}}})
		fp.Footprint.Existing = []application.ProviderFootprint{{InstanceID: "sql-a", MemoryBytes: fact.MemoryBytes}}
		fp.SnapshotRevision, err = guidedRevision(guidedRevisionSnapshot(application.ResolutionSnapshot{Target: d.Target, Facts: map[string]application.ProviderFact{"sql-a": fact}}))
		if err != nil {
			return nil, nil, err
		}
		return fp, impact, nil
	}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	w = appWizardAdvance(t, w, appWizardYes)
	if _, ok := w.Decision(); !ok {
		t.Fatal("observation-only refresh prevented decision")
	}
}

func TestAppWizardControllerValidityAuthorityChangesInvalidatePreview(t *testing.T) {
	for _, field := range []string{"expires-at", "meaning"} {
		t.Run(field, func(t *testing.T) {
			input := appWizardTestInput(t)
			calls := 0
			original := input.Refresh
			input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
				calls++
				fp, impact, err := original(ctx, d)
				if err != nil {
					return nil, nil, err
				}
				impact.Impacts = append(impact.Impacts, application.ProviderImpact{Materials: []application.MaterialConsequence{{Meaning: "within observed time window", Before: application.ProviderMaterial{ID: "certificate", ExpiresAt: "2036-11-13T05:39:05Z"}}}})
				materialImpact := len(impact.Impacts) - 1
				if calls > 1 {
					if field == "expires-at" {
						impact.Impacts[materialImpact].Materials[0].Before.ExpiresAt = "2036-11-12T05:39:05Z"
					} else {
						impact.Impacts[materialImpact].Materials[0].Meaning = "actual material validity has expired"
					}
				}
				return fp, impact, nil
			}
			w, err := newAppWizardController(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			w = appWizardAdvance(t, w, appWizardNext)
			if _, err := w.Step(context.Background(), appWizardYes); err == nil {
				t.Fatal("accepted changed absolute validity or expiry outcome")
			}
		})
	}
}

func TestAppWizardControllerProviderPreferenceChangeInvalidatesPreview(t *testing.T) {
	input := appWizardTestInput(t)
	calls := 0
	original := input.Refresh
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		calls++
		fp, impact, err := original(ctx, d)
		if err != nil {
			return nil, nil, err
		}
		if calls > 1 {
			fp.Preferences = []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "analytics"}, InstanceID: "sql-b", ProviderKind: capability.ProviderPostgreSQL}}
		}
		return fp, impact, nil
	}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if _, err := w.Step(context.Background(), appWizardYes); err == nil {
		t.Fatal("accepted changed provider preference")
	}
}

func TestAppWizardControllerProviderChangeRefreshesConsequences(t *testing.T) {
	w, err := newAppWizardController(context.Background(), appWizardTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if strings.Contains(w.View(), "sql-b: select") {
		t.Fatal("invented explicit choice")
	}
	w = appWizardAdvance(t, w, appWizardNo)
	w, err = w.SelectProvider(context.Background(), capability.Requirement{Kind: capability.SQL, Name: "analytics"}, "sql-b")
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if !strings.Contains(w.View(), "sql-b: select") {
		t.Fatal("preview not refreshed")
	}
}
func TestAppWizardControllerNonTTYNoInputAndInitiallyCancelled(t *testing.T) {
	for _, modify := range []func(*appWizardInput){
		func(i *appWizardInput) { i.TTY = false },
		func(i *appWizardInput) { i.NoInput = true },
	} {
		input := appWizardTestInput(t)
		modify(&input)
		if _, err := newAppWizardController(context.Background(), input); !errors.Is(err, errAppWizardNonInteractive) {
			t.Fatalf("unguarded interactive wizard: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newAppWizardController(ctx, appWizardTestInput(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAppWizardControllerPreviewIsNotLifecycleAuthorization(t *testing.T) {
	input := appWizardTestInput(t)
	plan := application.ProviderImpactPlan{Target: "dev", Destructive: true, ConfirmationsRequired: 2, Complete: false}
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		fp := &GuidedReadContext{}
		fp.Footprint.Target = d.Target
		plan.Target = d.Target
		return fp, &plan, nil
	}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	view := w.View()
	if !strings.Contains(view, "Provider change preview") || !strings.Contains(view, "2 confirmation") {
		t.Fatal(view)
	}
	if _, ok := w.Decision(); ok {
		t.Fatal("preview granted decision")
	}
	w = appWizardAdvance(t, w, appWizardYes)
	if _, ok := w.Decision(); !ok {
		t.Fatal("expected intent decision")
	}
	if !strings.Contains(w.View(), "execution requires separate authorized lifecycle") {
		t.Fatal("controller claimed execution")
	}
}

func TestAppWizardControllerNoInputMutationOutsideCorrectPhase(t *testing.T) {
	w, err := newAppWizardController(context.Background(), appWizardTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	before := w.selection.Requirements()
	if _, err := w.SelectProvider(context.Background(), capability.Requirement{Kind: capability.SQL, Name: "other"}, "sql-a"); err == nil {
		t.Fatal("nonselected provider allowed")
	}
	w = appWizardAdvance(t, w, appWizardNext)
	if _, err := w.SelectNeed(context.Background(), "space"); err == nil {
		t.Fatal("needs changed outside needs phase")
	}
	if !reflect.DeepEqual(w.selection.Requirements(), before) {
		t.Fatal("unexpected mutation")
	}
}

func TestAppWizardControllerDetachedNestedPreviewRejectsReusedPointer(t *testing.T) {
	input := appWizardTestInput(t)
	footprint := &GuidedReadContext{}
	footprint.Footprint.Target = input.Snapshot.Target
	impact := &application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion,
		Target:  input.Snapshot.Target,
		PlanID:  "original-plan",
		Impacts: []application.ProviderImpact{{
			InstanceID: "sql-a",
			Operation:  "replace",
			Functions: []application.FunctionConsequence{{
				Consumer: "api", Function: "database.query", Outcome: "continues",
			}},
		}},
	}
	calls := 0
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		calls++
		if calls == 2 {
			impact.PlanID = "changed-plan"
			impact.Impacts[0].Functions[0].Outcome = "unavailable"
		}
		return footprint, impact, nil
	}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	before := w.View()
	if !strings.Contains(before, "continues") || !strings.Contains(before, "original-plan") && w.impact.PlanID != "original-plan" {
		t.Fatal("missing initial authoritative preview")
	}
	impact.Impacts[0].Functions[0].Outcome = "externally modified"
	if strings.Contains(w.View(), "externally modified") || w.impact.PlanID != "original-plan" {
		t.Fatal("retained preview changed through external pointer")
	}
	next, err := w.Step(context.Background(), appWizardYes)
	if err == nil {
		t.Fatal("reused mutated projections incorrectly confirmed")
	}
	if _, ok := next.Decision(); ok {
		t.Fatal("stale data created decision")
	}
	if w.impact.PlanID != "original-plan" || w.impact.Impacts[0].Functions[0].Outcome != "continues" {
		t.Fatal("captured plan or nested consequences mutated")
	}
}

func TestAppWizardControllerProviderRowsUseRealNewlines(t *testing.T) {
	w, err := newAppWizardController(context.Background(), appWizardTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	view := w.View()
	if strings.Contains(view, `\nSQL`) || strings.Contains(view, `\n[x]`) {
		t.Fatalf("literal backslash separators in view: %q", view)
	}
	if !strings.Contains(view, "\n"+string(capability.SQL)+"/primary") || !strings.Contains(view, "\n"+string(capability.SQL)+"/analytics") {
		t.Fatalf("provider choices not on distinct lines: %q", view)
	}
}

func TestAppWizardControllerFreshFactsUsesBoundedRuntimeTimeout(t *testing.T) {
	input := appWizardTestInput(t)
	var remaining time.Duration
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("preview refresh has no deadline")
		}
		remaining = time.Until(deadline)
		return nil, nil, errors.New("stop after inspecting preview deadline")
	}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = w.freshFacts(context.Background(), w.intent())
	if err == nil || err.Error() != "stop after inspecting preview deadline" {
		t.Fatalf("freshFacts error = %v", err)
	}
	if remaining < 119*time.Second || remaining > appWizardPreviewRefreshTimeout {
		t.Fatalf("preview refresh timeout budget = %s, want about %s", remaining, appWizardPreviewRefreshTimeout)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	w, err = newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = w.freshFacts(canceled, w.intent())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("freshFacts error after caller cancellation = %v, want context.Canceled", err)
	}
}

func TestAppWizardControllerFormatting(t *testing.T) {
	for _, name := range []string{"app_wizard_controller.go", "app_wizard_controller_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) != 0 {
			t.Errorf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}
