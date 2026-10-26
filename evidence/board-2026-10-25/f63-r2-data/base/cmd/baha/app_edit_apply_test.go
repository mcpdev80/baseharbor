package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appEditTestLoaded(t *testing.T, target string, requirements []capability.Requirement, preferences []application.ProviderPreference) appEditLoaded {
	t.Helper()
	manifest := application.New("edit-safe", "dev", false, false, false)
	for _, requirement := range requirements {
		switch requirement.Kind {
		case capability.SQL:
			manifest = application.WithSQLInstances(manifest, requirement.Name)
		case capability.KeyValue:
			manifest = application.WithCacheInstances(manifest, requirement.Name)
		case capability.DurableKeyValue:
			manifest = application.WithKeyValueInstances(manifest, requirement.Name)
		case capability.DocumentDatabase:
			manifest = application.WithDocumentDatabaseInstances(manifest, requirement.Name)
		case capability.MessagingQueue:
			manifest = application.WithMessagingQueueInstances(manifest, requirement.Name)
		case capability.ObjectStorageS3:
			manifest = application.WithObjectStorageBuckets(manifest, requirement.Name)
		}
	}
	manifest = application.WithWorkloadComponents(manifest, "api")
	manifest = application.WithHA(manifest, true)
	return appEditLoaded{
		Source:        []byte("# exact authored comment\n" + manifest.AuthoredYAML()),
		Target:        target,
		ApplicationID: manifest.ApplicationID,
		Capabilities:  append([]capability.Requirement(nil), requirements...),
		Preferences:   append([]application.ProviderPreference(nil), preferences...),
		Complete:      true,
	}
}

func appEditTestController(target string, requirements []capability.Requirement, preferences []application.ProviderPreference, plan application.ProviderImpactPlan) appWizardController {
	decision := appWizardDecision{Target: target, Requirements: append([]capability.Requirement(nil), requirements...)}
	for _, preference := range preferences {
		decision.Preferences = append(decision.Preferences, appProviderPreference{Target: target, ProviderPreference: preference})
	}
	return appWizardController{phase: appWizardDecided, decision: &decision, impact: &plan}
}

func appEditTestAuthorities(target, applicationID string, requirements []capability.Requirement, preferences []application.ProviderPreference, planSequence []application.ProviderImpactPlan, execute *int) appEditAuthorities {
	revision := "snapshot-1"
	read := func(context.Context, string) (GuidedReadContext, error) {
		return GuidedReadContext{
			ApplicationID: applicationID,
			Name:          "edit-safe",
			Environment:   "dev",
			Target:        target,
			Capabilities:  append([]capability.Requirement(nil), requirements...),
			Preferences:   append([]application.ProviderPreference(nil), preferences...),
			Footprint: application.Footprint{
				Version: application.FootprintVersion,
				Target:  target,
			},
			Complete:         true,
			SnapshotRevision: revision,
		}, nil
	}
	choices := func(_ context.Context, gotTarget string, requested []capability.Requirement) ([]appProviderChoice, error) {
		out := make([]appProviderChoice, 0, len(requested))
		for _, requirement := range requested {
			id := "provider-" + string(requirement.Kind) + "-" + requirement.Name
			candidates := []appProviderCandidate{{Instance: capability.ProviderInstance{ID: id}}}
			for _, preference := range preferences {
				if preference.Requirement == requirement && preference.InstanceID != "" {
					candidates = append(candidates, appProviderCandidate{Instance: capability.ProviderInstance{ID: preference.InstanceID}})
				}
			}
			out = append(out, appProviderChoice{
				Target:             gotTarget,
				Requirement:        requirement,
				ProposedInstanceID: id,
				Candidates:         candidates,
			})
		}
		return out, nil
	}
	planIndex := 0
	planner := func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error) {
		if len(planSequence) == 0 {
			return application.ProviderImpactPlan{}, errors.New("missing test plan")
		}
		index := planIndex
		if index >= len(planSequence) {
			index = len(planSequence) - 1
		}
		planIndex++
		return planSequence[index], nil
	}
	return appEditAuthorities{
		Read:    read,
		Choices: choices,
		Plan:    planner,
		Authorize: func(_ context.Context, gotTarget, gotApplicationID string) error {
			if gotTarget != target || gotApplicationID != applicationID {
				return errors.New("foreign owner")
			}
			return nil
		},
		Execute: func(_ context.Context, _ appEditDesired, plan application.ProviderImpactPlan, _ application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			(*execute)++
			outcomes := make([]application.ProviderExecutionOutcome, 0, len(plan.Impacts))
			for _, impact := range plan.Impacts {
				outcomes = append(outcomes, application.ProviderExecutionOutcome{InstanceID: impact.InstanceID, Operation: impact.Operation, Status: "applied", Verified: true, Evidence: "test execution receipt"})
			}
			return outcomes, nil
		},
	}
}

func TestAppEditApplyNoChangeAndCancelledWriteNothing(t *testing.T) {
	requirements := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	loaded := appEditTestLoaded(t, "dev", requirements, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r1", PlanID: "same", ConfirmationsRequired: 1}
	executed := 0
	authorities := appEditTestAuthorities("dev", loaded.ApplicationID, requirements, nil, []application.ProviderImpactPlan{plan}, &executed)
	controller := appEditTestController("dev", requirements, nil, plan)

	result, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{}, authorities)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || executed != 0 || !bytes.Equal(result.Source, loaded.Source) {
		t.Fatalf("no-op changed source or executed mutation: %+v calls=%d", result, executed)
	}

	cancelled := controller.abort()
	if _, err := appEditApplyConfirmed(context.Background(), cancelled, loaded, application.ProviderChangeConsent{}, authorities); !errors.Is(err, errAppWizardAbort) {
		t.Fatalf("cancelled edit error = %v", err)
	}
	if executed != 0 || !bytes.Equal(appEditLoadSource(loaded), loaded.Source) {
		t.Fatal("cancelled edit reached mutation or changed exact source")
	}
}

func TestAppEditApplyStalePlanAndWrongConsentWriteNothing(t *testing.T) {
	before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	after := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}, {Kind: capability.KeyValue, Name: "cache"}}
	loaded := appEditTestLoaded(t, "dev", before, nil)
	first := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r1", PlanID: "plan-1", ConfirmationsRequired: 1, Impacts: []application.ProviderImpact{{InstanceID: "cache", Operation: "add"}}}
	stale := first
	stale.PlanID, stale.Revision = "plan-2", "r2"
	executed := 0
	controller := appEditTestController("dev", after, nil, first)
	authorities := appEditTestAuthorities("dev", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{first, stale}, &executed)
	_, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: first.PlanID, First: true}, authorities)
	var machineErr *appMachineError
	if !errors.As(err, &machineErr) || machineErr.Code != "conflict" {
		t.Fatalf("stale plan error = %v", err)
	}
	if executed != 0 {
		t.Fatal("stale plan executed mutation")
	}

	executed = 0
	authorities = appEditTestAuthorities("dev", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{first}, &executed)
	_, err = appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: "wrong", First: true}, authorities)
	if !errors.As(err, &machineErr) || machineErr.Code != "conflict" {
		t.Fatalf("wrong consent error = %v", err)
	}
	if executed != 0 {
		t.Fatal("wrong consent executed mutation")
	}
}

func TestGuidedAppEditRefusalAndPartialResult(t *testing.T) {
	t.Run("refusal-without-execution", TestAppEditApplyRefusalHasNoExecutionResult)
	t.Run("authorization-and-partial-execution", TestAppEditApplyForeignOwnerAndPartialActualResult)
	t.Run("destructive-and-shared-admission", TestAppEditApplyRemovalRespectsDestructiveAndSharedPlanSemantics)
}

func TestAppEditApplyRefusalHasNoExecutionResult(t *testing.T) {
	before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	after := append(append([]capability.Requirement(nil), before...), capability.Requirement{Kind: capability.KeyValue, Name: "cache"})
	loaded := appEditTestLoaded(t, "dev", before, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r1", PlanID: "refusal-plan", Complete: true, ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{{InstanceID: "cache", Operation: "add"}}}
	for _, scenario := range []string{"missing-consent", "authorization", "fresh-read"} {
		t.Run(scenario, func(t *testing.T) {
			executed := 0
			authorities := appEditTestAuthorities("dev", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{plan}, &executed)
			consent := application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}
			wantCode := "denied"
			switch scenario {
			case "missing-consent":
				consent.First = false
				wantCode = "approval_required"
			case "authorization":
				authorities.Authorize = func(context.Context, string, string) error { return errors.New("current actor denied") }
			case "fresh-read":
				read := authorities.Read
				reads := 0
				authorities.Read = func(ctx context.Context, target string) (GuidedReadContext, error) {
					reads++
					if reads == 3 {
						return GuidedReadContext{}, &GuidedReadError{Kind: "denied"}
					}
					return read(ctx, target)
				}
			}
			result, err := appEditApplyConfirmed(context.Background(), appEditTestController("dev", after, nil, plan), loaded, consent, authorities)
			var machineErr *appMachineError
			if !errors.As(err, &machineErr) || machineErr.Code != wantCode {
				t.Fatalf("refusal classification: got %v, want %s", err, wantCode)
			}
			var resultErr *application.ProviderChangeError
			if errors.As(err, &resultErr) {
				t.Fatalf("pre-execution refusal acquired an execution-result error: %v", err)
			}
			if executed != 0 || result.Changed || result.Presentation != "" || !bytes.Equal(result.Source, loaded.Source) ||
				!reflect.DeepEqual(result.Result, application.ProviderChangeResult{}) {
				t.Fatalf("pre-execution refusal claimed a change or receipt: executions=%d result=%+v", executed, result)
			}
		})
	}
}

func TestAppEditApplyForeignOwnerAndPartialActualResult(t *testing.T) {
	before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	after := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}, {Kind: capability.KeyValue, Name: "cache"}}
	loaded := appEditTestLoaded(t, "prod", before, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "prod", Revision: "r1", PlanID: "plan-partial", ConfirmationsRequired: 1, Impacts: []application.ProviderImpact{
		{InstanceID: "cache", Operation: "add"},
		{InstanceID: "old-sql", Operation: "stop"},
	}}
	controller := appEditTestController("prod", after, nil, plan)
	executed := 0
	authorities := appEditTestAuthorities("prod", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{plan}, &executed)
	authorities.Authorize = func(context.Context, string, string) error { return errors.New("foreign owner") }
	_, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
	if err == nil || executed != 0 {
		t.Fatalf("foreign owner reached mutation: err=%v calls=%d", err, executed)
	}

	executed = 0
	authorities = appEditTestAuthorities("prod", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{plan}, &executed)
	executionFailure := errors.New("old SQL stop failed")
	authorities.Execute = func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		return []application.ProviderExecutionOutcome{{InstanceID: "cache", Operation: "add", Status: "applied", Verified: true, Evidence: "cache add verified"}}, executionFailure
	}
	result, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
	var partial *appMachineError
	if !errors.As(err, &partial) || partial.Code != "partial" {
		t.Fatalf("partial execution error = %v", err)
	}
	if executed != 1 || !result.Changed || !errors.Is(err, executionFailure) || result.Result.Version != application.ProviderImpactVersion || result.Result.PlanID != plan.PlanID || result.Result.Status != "partial" || !strings.Contains(result.Presentation, "old-sql: stop") || !strings.Contains(result.Presentation, "fresh consent") {
		t.Fatalf("actual partial result not presented: calls=%d result=%+v text=%s", executed, result.Result, result.Presentation)
	}
}

func TestAppEditApplyTargetPreferencesAndPreservedIntent(t *testing.T) {
	before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	after := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}, {Kind: capability.KeyValue, Name: "cache"}}
	for _, target := range []string{"dev", "prod"} {
		t.Run(target, func(t *testing.T) {
			loadedPreference := application.ProviderPreference{Requirement: before[0], InstanceID: "sql-" + target}
			loaded := appEditTestLoaded(t, target, before, []application.ProviderPreference{loadedPreference})
			decisionPreference := loadedPreference
			plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target, Revision: "r", PlanID: "p-" + target, ConfirmationsRequired: 1, Impacts: []application.ProviderImpact{{InstanceID: "cache", Operation: "add"}}}
			controller := appEditTestController(target, after, []application.ProviderPreference{decisionPreference}, plan)
			executed := 0
			authorities := appEditTestAuthorities(target, loaded.ApplicationID, before, []application.ProviderPreference{loadedPreference}, []application.ProviderImpactPlan{plan}, &executed)
			authorities.Execute = func(_ context.Context, desired appEditDesired, _ application.ProviderImpactPlan, _ application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
				executed++
				if desired.Target != target || desired.Preferences[0].InstanceID != "sql-"+target {
					t.Fatalf("Target preference lost: %+v", desired.Preferences)
				}
				if !desired.Manifest.HA || !reflect.DeepEqual(application.WorkloadComponentNames(desired.Manifest), []string{"api"}) {
					t.Fatalf("non-edited intent lost: %+v", desired.Manifest)
				}
				return []application.ProviderExecutionOutcome{{InstanceID: "cache", Operation: "add", Status: "applied", Verified: true, Evidence: "cache add verified"}}, nil
			}
			_, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
			if err != nil {
				t.Fatal(err)
			}
			if executed != 1 {
				t.Fatalf("execute calls = %d", executed)
			}
		})
	}
}

func TestAppEditApplyProviderSwitchUsesConfirmedTargetPreference(t *testing.T) {
	requirement := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	before := []capability.Requirement{requirement}
	loadedPreference := application.ProviderPreference{Requirement: requirement, InstanceID: "sql-old"}
	// The confirmed switch must target a genuine Target candidate produced by the
	// accepted choice projection ("provider-" + capability.SQL + "-" + name), not a
	// fabricated identifier, so the real choice authority resolves the transition.
	confirmedPreference := application.ProviderPreference{Requirement: requirement, InstanceID: "provider-" + string(capability.SQL) + "-" + requirement.Name}
	loaded := appEditTestLoaded(t, "prod", before, []application.ProviderPreference{loadedPreference})
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, Target: "prod", Revision: "r-switch", PlanID: "plan-switch",
		Complete: true, ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{{InstanceID: "sql-old", Operation: "replace"}},
	}
	controller := appEditTestController("prod", before, []application.ProviderPreference{confirmedPreference}, plan)
	executed := 0
	authorities := appEditTestAuthorities("prod", loaded.ApplicationID, before, []application.ProviderPreference{loadedPreference}, []application.ProviderImpactPlan{plan}, &executed)
	authorities.Execute = func(_ context.Context, desired appEditDesired, gotPlan application.ProviderImpactPlan, _ application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		if len(desired.Preferences) != 1 || desired.Preferences[0] != confirmedPreference {
			t.Fatalf("confirmed provider switch lost: %+v", desired.Preferences)
		}
		if len(gotPlan.Impacts) != 1 || gotPlan.Impacts[0].Operation != "replace" || gotPlan.Impacts[0].InstanceID != "sql-old" {
			t.Fatalf("provider switch plan changed: %+v", gotPlan.Impacts)
		}
		return []application.ProviderExecutionOutcome{{InstanceID: "sql-old", Operation: "replace", Status: "applied", Verified: true, Evidence: "provider switch verified"}}, nil
	}
	result, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
	if err != nil {
		t.Fatal(err)
	}
	if executed != 1 || result.Result.Status != "complete" {
		t.Fatalf("provider switch result: calls=%d result=%+v", executed, result.Result)
	}
}

func TestAppEditApplyRemovalRespectsDestructiveAndSharedPlanSemantics(t *testing.T) {
	sql := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	cache := capability.Requirement{Kind: capability.KeyValue, Name: "cache"}
	before := []capability.Requirement{sql, cache}
	after := []capability.Requirement{sql}
	loaded := appEditTestLoaded(t, "prod", before, nil)

	destructive := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, Target: "prod", Revision: "r-destroy", PlanID: "plan-destroy",
		Complete: true, ConfirmationsRequired: 2, Destructive: true,
		Impacts: []application.ProviderImpact{{InstanceID: "cache", Operation: "destroy"}},
	}
	controller := appEditTestController("prod", after, nil, destructive)
	executed := 0
	authorities := appEditTestAuthorities("prod", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{destructive}, &executed)
	_, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: destructive.PlanID, First: true, Second: true}, authorities)
	var machineErr *appMachineError
	if !errors.As(err, &machineErr) || machineErr.Code != "approval_required" || executed != 0 {
		t.Fatalf("destructive removal without data consent: err=%v calls=%d", err, executed)
	}

	authorities = appEditTestAuthorities("prod", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{destructive}, &executed)
	authorities.Execute = func(_ context.Context, _ appEditDesired, gotPlan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		if !consent.DeleteData || gotPlan.Impacts[0].Operation != "destroy" {
			t.Fatalf("destructive consent/plan changed: consent=%+v plan=%+v", consent, gotPlan.Impacts)
		}
		return []application.ProviderExecutionOutcome{{InstanceID: "cache", Operation: "destroy", Status: "applied", Verified: true, Evidence: "destructive removal verified"}}, nil
	}
	if _, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: destructive.PlanID, First: true, Second: true, DeleteData: true}, authorities); err != nil {
		t.Fatal(err)
	}
	if executed != 1 {
		t.Fatalf("destructive removal execute calls = %d", executed)
	}

	shared := destructive
	shared.Revision = "r-unbind"
	shared.PlanID = "plan-unbind"
	shared.Destructive = false
	shared.ConfirmationsRequired = 1
	shared.Impacts = []application.ProviderImpact{{InstanceID: "shared-cache", Operation: "unbind"}}
	controller = appEditTestController("prod", after, nil, shared)
	executed = 0
	authorities = appEditTestAuthorities("prod", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{shared}, &executed)
	authorities.Execute = func(_ context.Context, _ appEditDesired, gotPlan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		if consent.DeleteData || len(gotPlan.Impacts) != 1 || gotPlan.Impacts[0].Operation != "unbind" {
			t.Fatalf("shared-provider plan was escalated: consent=%+v plan=%+v", consent, gotPlan.Impacts)
		}
		return []application.ProviderExecutionOutcome{{InstanceID: "shared-cache", Operation: "unbind", Status: "applied", Verified: true, Evidence: "shared binding removal verified"}}, nil
	}
	if _, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: shared.PlanID, First: true}, authorities); err != nil {
		t.Fatal(err)
	}
	if executed != 1 {
		t.Fatalf("shared unbind execute calls = %d", executed)
	}
}

// TestReview918DriverEditsNeedsFromInput proves the delivered interactive handler
// actually edits needs: a space toggles the focused need, then navigation and
// confirmation reflect the changed selection rather than re-confirming it.
func TestAppEditCommandReview918DriverEditsNeedsFromInput(t *testing.T) {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	loaded := appEditTestLoaded(t, "dev", []capability.Requirement{req}, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", Complete: true, ConfirmationsRequired: 0}
	controller, err := newAppWizardController(context.Background(), appWizardInput{
		Manifest:  mustAppEditManifest(t, loaded),
		Snapshot:  application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{req}},
		TTY:       true,
		Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			view := &GuidedReadContext{ApplicationID: loaded.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
			copyPlan := plan
			return view, &copyPlan, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := appEditSession{Loaded: loaded, Controller: controller}
	// Real input bytes: a single space toggles the focused need, a bare Enter
	// advances to the preview, then yes confirms.
	session, err = driveAppEditWizard(context.Background(), session, bufioReader(" \n\nyes\n"), io.Discard)
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	decision, confirmed := session.Controller.Decision()
	if !confirmed {
		t.Fatal("edit driver did not reach a confirmed decision")
	}
	for _, got := range decision.Requirements {
		if got == req {
			t.Fatalf("space did not toggle the focused need; still selected: %+v", decision.Requirements)
		}
	}
}

// TestReview918DriverEditsProviderFromInput proves provider choices are editable:
// a real candidate instance from the Target registry is carried into the decision.
func TestAppEditCommandReview918DriverEditsProviderFromInput(t *testing.T) {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	loaded := appEditTestLoaded(t, "dev", []capability.Requirement{req}, nil)
	manifest := mustAppEditManifest(t, loaded)
	registry := capability.NewRegistry()
	registry.Instances = append(registry.Instances, capability.ProviderInstance{
		ID: "sql/shared", Provider: capability.PostgreSQL,
		Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor,
	})
	snapshot := application.ResolutionSnapshot{Target: "dev", Registry: registry, Facts: map[string]application.ProviderFact{}}
	selection := newAppCapabilitySelection(appCapabilitySelectionInput{Current: []capability.Requirement{req}})
	choices, err := appProviderChoices(manifest, snapshot, selection)
	if err != nil || len(choices) == 0 || len(choices[0].Candidates) == 0 {
		t.Fatalf("no provider candidates: %v", err)
	}
	instanceID := choices[0].Candidates[0].Instance.ID
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", Complete: true}
	controller, err := newAppWizardController(context.Background(), appWizardInput{
		Manifest: manifest, Snapshot: snapshot,
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{req}}, TTY: true,
		Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			view := &GuidedReadContext{ApplicationID: manifest.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
			copyPlan := plan
			return view, &copyPlan, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := appEditSession{Loaded: loaded, Controller: controller}
	command := "provider " + string(capability.SQL) + "/" + req.Name + " " + instanceID + "\n"
	session, err = driveAppEditWizard(context.Background(), session, bufioReader(command+"next\nyes\n"), io.Discard)
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	decision, confirmed := session.Controller.Decision()
	if !confirmed {
		t.Fatal("provider edit did not confirm")
	}
	for _, p := range decision.Preferences {
		if p.Requirement == req && p.InstanceID == instanceID {
			return
		}
	}
	t.Fatalf("provider selection lost: %+v", decision.Preferences)
}

func TestAppEditCommandProviderInputReturnsToAutomatic(t *testing.T) {
	for _, test := range []struct {
		input   string
		want    string
		invalid bool
	}{
		{input: "automatic"},
		{input: "AuToMaTiC"},
		{input: "missing-instance", want: "sql-a", invalid: true},
	} {
		t.Run(test.input, func(t *testing.T) {
			input := appWizardTestInput(t)
			// One compatible provider keeps Automatic unambiguous. Each named
			// requirement must retain its own independent Target preference.
			input.Snapshot.Registry.Instances = input.Snapshot.Registry.Instances[:1]
			controller, err := newAppWizardController(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			command := "provider database.sql/primary sql-a\nprovider database.sql/analytics sql-a\nnext\nno\n" +
				"provider database.sql/primary " + test.input + "\nnext\nyes\n"
			var out bytes.Buffer
			session, err := driveAppEditWizard(context.Background(), appEditSession{Controller: controller}, bufioReader(command), &out)
			if err != nil {
				t.Fatalf("drive Edit input: %v", err)
			}
			decision, confirmed := session.Controller.Decision()
			if !confirmed || decision.Target != "dev" || len(decision.Preferences) != 2 {
				t.Fatalf("Edit input lost its confirmed Target preferences: %+v", decision)
			}
			for _, preference := range decision.Preferences {
				want := "sql-a"
				if preference.Requirement.Name == "primary" {
					want = test.want
				}
				if preference.Target != "dev" || preference.InstanceID != want {
					t.Fatalf("provider input %q yielded %+v, want instance %q", test.input, preference, want)
				}
			}
			if strings.Contains(out.String(), "provider selection failed") != test.invalid {
				t.Fatalf("unexpected provider input diagnostic: %s", out.String())
			}
		})
	}
}

// TestReview918RunAppEditCancelsWriteFree calls the assembled handler end-to-end
// against a registered existing application. A cancel must reach the input path,
// return the cancel error, and produce no lifecycle run and no source write.
func TestAppEditCommandReview918RunAppEditCancelsWriteFree(t *testing.T) {
	target := configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n")
	m := application.New("edit-cancel", "dev", false, false, false)
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, manifestPath, m)
	registerTestDeployment(t, target, m, root, manifestPath)

	sourceBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	prevRun, prevObserve := applicationApplyLifecycleRunWithHooks, observeApplicationProviderFactsFunc
	t.Cleanup(func() {
		applicationApplyLifecycleRunWithHooks, observeApplicationProviderFactsFunc = prevRun, prevObserve
	})
	observeApplicationProviderFactsFunc = func(context.Context, resolvedApplication) map[string]application.ProviderFact {
		return map[string]application.ProviderFact{}
	}
	applicationApplyLifecycleRunWithHooks = func(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, hooks applicationApplyLifecycleHooks) (applicationApplyLifecycleEvidence, error) {
		runs++
		return prevRun(ctx, store, args, out, errOut, hooks)
	}

	for _, input := range []string{"cancel\n", ""} {
		_, err = runAppEdit(context.Background(), application.DefaultStore(), []string{"edit-cancel"}, strings.NewReader(input), io.Discard, io.Discard)
		if !errors.Is(err, errAppWizardAbort) {
			t.Fatalf("cancel/EOF path %q error = %v, want %v", input, err, errAppWizardAbort)
		}
		if runs != 0 {
			t.Fatalf("cancel/EOF path %q reached the lifecycle %d times", input, runs)
		}
		sourceAfter, _ := os.ReadFile(manifestPath)
		if !bytes.Equal(sourceBefore, sourceAfter) {
			t.Fatalf("cancel/EOF path %q modified application source", input)
		}
	}
}

// TestReview918SpaceAndEnterBytesAreDistinct confirms the classifier preserves
// whitespace before deciding: a real space byte toggles the focused need while a
// bare Enter (empty line) only advances and leaves the selection intact.
func TestAppEditCommandReview918SpaceAndEnterBytesAreDistinct(t *testing.T) {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	loaded := appEditTestLoaded(t, "dev", []capability.Requirement{req}, nil)
	manifest := mustAppEditManifest(t, loaded)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", Complete: true}
	build := func() appEditSession {
		controller, err := newAppWizardController(context.Background(), appWizardInput{
			Manifest: manifest, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
			Selection: appCapabilitySelectionInput{Current: []capability.Requirement{req}}, TTY: true,
			Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
				view := &GuidedReadContext{ApplicationID: manifest.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
				copyPlan := plan
				return view, &copyPlan, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return appEditSession{Loaded: loaded, Controller: controller}
	}

	// Bare Enter only: the selected need is re-confirmed unchanged.
	enterOnly, _ := driveAppEditWizard(context.Background(), build(), bufioReader("\nyes\n"), io.Discard)
	if _, confirmed := enterOnly.Controller.Decision(); !confirmed {
		t.Fatal("bare-Enter run did not confirm")
	}
	selected := false
	for _, got := range enterOnly.Controller.decision.Requirements {
		if got == req {
			selected = true
		}
	}
	if !selected {
		t.Fatalf("bare Enter must not toggle the need: %+v", enterOnly.Controller.decision.Requirements)
	}

	// A single space byte toggles the focused need off; Enter then advances.
	spaced, _ := driveAppEditWizard(context.Background(), build(), bufioReader(" \n\nyes\n"), io.Discard)
	if _, confirmed := spaced.Controller.Decision(); !confirmed {
		t.Fatal("space run did not confirm")
	}
	for _, got := range spaced.Controller.decision.Requirements {
		if got == req {
			t.Fatalf("real space byte did not toggle the need: %+v", spaced.Controller.decision.Requirements)
		}
	}
}

func TestAppEditCommandReview918PersistExactTargetPreferenceAndResolveLater(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	m := application.WithSQLInstances(application.New("edit-preference", "dev", true, false, false), "primary")
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	selectedManifest, err := application.WithBackendPlacementProjection(m, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeShared, SharingBoundary: "team:edit-choice", Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := application.PreviewReferenceProviderRegistryAt(root, selectedManifest)
	if err != nil {
		t.Fatal(err)
	}
	var selected capability.ProviderInstance
	for _, instance := range projected.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeShared {
			selected = instance
		}
	}
	if selected.ID == "" {
		t.Fatal("no canonical selected provider instance")
	}
	registry := capability.NewRegistry()
	if err := registry.Register(selected); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	t.Chdir(parent)
	mustWriteWizardTestFile(t, filepath.Join(parent, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n")
	path := filepath.Join(parent, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, path, m)
	registerTestDeployment(t, target, m, parent, path)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{"edit-preference"}, "edit")
	if err != nil {
		t.Fatal(err)
	}
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	pref := application.ProviderPreference{Requirement: req, InstanceID: selected.ID, ProviderKind: selected.Provider.Kind}
	desired := appEditDesired{
		Source: source, Manifest: m, Target: target.Name, ApplicationID: m.ApplicationID,
		Requirements: []capability.Requirement{req}, Preferences: []application.ProviderPreference{pref},
	}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, PlanID: "edit-exact-choice", Revision: "r1", ConfirmationsRequired: 1}
	calls := 0
	authorities := appEditTestAuthorities(target.Name, m.ApplicationID, []capability.Requirement{req}, nil, []application.ProviderImpactPlan{plan}, &calls)
	authorities.Choices = func(_ context.Context, gotTarget string, _ []capability.Requirement) ([]appProviderChoice, error) {
		return []appProviderChoice{{Target: gotTarget, Requirement: req, ProposedInstanceID: selected.ID, Candidates: []appProviderCandidate{{Instance: selected}}}}, nil
	}
	executor := appEditRuntimeExecuteAuthority(application.DefaultStore(), []string{"edit-preference"}, resolved, source, authorities, io.Discard, io.Discard, nil,
		func(ctx context.Context, _ application.Store, _ []string, _ application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, _, _ io.Writer) ([]application.ProviderExecutionOutcome, error) {
			calls++
			return nil, mutate(ctx, resolved)
		})
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: "stale-plan", First: true}); err == nil {
		t.Fatal("stale consent unexpectedly accepted")
	}
	notStored, err := application.DesiredProviderPreferencesAt(root, m)
	if err != nil || len(notStored) != 0 {
		t.Fatalf("stale consent wrote desired provider preference: %+v %v", notStored, err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, source) {
		t.Fatalf("stale consent wrote portable source: %v", err)
	}
	calls = 0
	if _, err := executor(context.Background(), desired, plan, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one mutation invocation, got %d", calls)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, source) {
		t.Fatalf("preference-only Edit rewrote portable source: %v", err)
	}
	saved, err := application.DesiredProviderPreferencesAt(root, m)
	if err != nil || len(saved) != 1 || saved[0] != pref {
		t.Fatalf("confirmed preference not stored by canonical owner: %+v %v", saved, err)
	}
	afterResolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{"edit-preference"}, "plan")
	if err != nil {
		t.Fatal(err)
	}
	placement, err := application.ResolveProviderPlacement(afterResolved.Manifest, capability.ProviderPostgreSQL)
	if err != nil || placement.SharingBoundary != selected.SharingBoundary {
		t.Fatalf("later app resolution ignored the selected instance: %+v %v", placement, err)
	}
}

func TestAppEditCommandIncompleteSQLConsumerRemainsPartial(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	m := application.New("edit-runtime-integration", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n")
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, target, m, root, sourcePath)

	desired := application.WithSQLInstances(m, "default")
	registry, err := application.PreviewReferenceProviderRegistryAt(stateRoot, desired)
	if err != nil {
		t.Fatal(err)
	}
	var selected capability.ProviderInstance
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication {
			selected = instance
		}
	}
	if selected.ID == "" {
		t.Fatalf("SQL selection lacks an application-owned canonical provider instance: %+v", registry.Instances)
	}
	// Make the instance selectable without pretending the new SQL requirement
	// is already bound before the real Edit lifecycle executes.
	available := capability.NewRegistry()
	if err := available.Register(selected); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(available); err != nil {
		t.Fatal(err)
	}

	project := bhruntime.ApplicationProjectName(target.Name, m.Name, m.Environment)
	t.Cleanup(func() {
		output, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+project).Output()
		if err == nil && len(strings.Fields(string(output))) > 0 {
			args := append([]string{"rm", "-fv"}, strings.Fields(string(output))...)
			_ = exec.Command("docker", args...).Run()
		}
	})
	input := strings.NewReader(" \nprovider database.sql/default " + selected.ID + "\nnext\nyes\ny\ny\ny\n")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var out bytes.Buffer
	result, err := runAppEdit(ctx, application.DefaultStore(), []string{m.Name}, input, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("unobserved SQL consumer/data changes must remain partial: result=%+v err=%v\n%s", result, err, out.String())
	}
	if result.Result.Status == "complete" {
		t.Fatal("unobserved SQL change returned a complete receipt")
	}
}

func TestAppEditCommandCompleteProviderEditRunsRealLifecycle(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	// Hosted CI provides an explicitly selected rootful Docker daemon, not the
	// default rootless socket. Never fall back after a failed engine selection.
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	target := configureTestTarget(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	definition := cfg.Targets[target.Name]
	definition.Runtime.DockerMode = "rootful"
	definition.Runtime.DockerEndpoint = "unix:///var/run/docker.sock"
	cfg.Targets[target.Name] = definition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err = cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	m := application.New("edit-runtime-integration", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n")
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, target, m, root, sourcePath)
	// Establish the real previous workload-only deployment. Its retained
	// canonical native definition and exact runtime inventory must prove
	// absence of a SQL provider before the confirmed Edit creates one.
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer setupCancel()
	if err := executeApplicationApplyLifecycle(setupCtx, application.DefaultStore(), []string{m.Name}, io.Discard, io.Discard); err != nil {
		t.Fatalf("baseline workload-only lifecycle failed: %v", err)
	}

	desired := application.WithSQLInstances(m, "default")
	registry, err := application.PreviewReferenceProviderRegistryAt(stateRoot, desired)
	if err != nil {
		t.Fatal(err)
	}
	var selected capability.ProviderInstance
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication {
			selected = instance
		}
	}
	if selected.ID == "" {
		t.Fatalf("SQL selection lacks an application-owned canonical provider instance: %+v", registry.Instances)
	}
	// Make the instance selectable without pretending the new SQL requirement
	// is already bound before the real Edit lifecycle executes.
	available := capability.NewRegistry()
	if err := available.Register(selected); err != nil {
		t.Fatal(err)
	}
	// Retain the canonical binding of a stopped, never-realized SQL provider.
	// This is a supported empty-provider resume, not a data migration or a
	// fabricated runtime receipt; the actual daemon still has no SQL volume.
	resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.SQL, Name: "default"}, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if err := available.BindDeployment(resource, m.ApplicationID, m.Environment, selected.ID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}).Save(available); err != nil {
		t.Fatal(err)
	}

	project := bhruntime.ApplicationProjectName(target.Name, m.Name, m.Environment)
	t.Cleanup(func() {
		output, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+project).Output()
		if err == nil && len(strings.Fields(string(output))) > 0 {
			args := append([]string{"rm", "-fv"}, strings.Fields(string(output))...)
			_ = exec.Command("docker", args...).Run()
		}
	})
	oldSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(" \nprovider database.sql/default " + selected.ID + "\nnext\nyes\ny\ny\ny\n")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var out bytes.Buffer
	result, err := runAppEdit(ctx, application.DefaultStore(), []string{m.Name}, input, &out, &out)
	if err != nil {
		afterResolved, resolveErr := resolveApplication(context.Background(), application.DefaultStore(), []string{m.Name}, "plan")
		if resolveErr == nil {
			t.Logf("measured post-Edit fact: %+v", observeApplicationProviderFacts(context.Background(), afterResolved)[selected.ID])
			t.Logf("native selected SQL registry: %+v", afterResolved.Manifest.Services)
			if nativeRuntime, runtimeErr := detectRuntimeForTarget(ctx, target); runtimeErr == nil {
				files := application.RuntimeFilesFor(application.DefaultStore(), afterResolved.Manifest)
				inventory, probeErr := application.ObserveSQLConsumerInventory(ctx, application.NewRuntimeBackendProbeExecutor(nativeRuntime, files), files, afterResolved.Manifest, "default")
				t.Logf("actual SQL consumer inventory: %+v probe=%v", inventory, probeErr)
			} else {
				t.Logf("native SQL runtime unavailable for post-Edit probe: %v", runtimeErr)
			}
		} else {
			t.Logf("post-Edit application resolution failed: %v", resolveErr)
		}
		t.Fatalf("real Edit lifecycle did not converge: %v\nplan=%+v\nresult=%+v\n%s", err, result.Plan, result.Result, out.String())
	}
	if !result.Changed || result.Presentation == "" || result.Result.Status != "complete" {
		t.Fatalf("confirmed real edit did not return an execution result: %+v", result)
	}
	applied := false
	for _, outcome := range result.Result.Outcomes {
		if outcome.InstanceID == selected.ID && (outcome.Operation == "add" || outcome.Operation == "resume") &&
			outcome.Status == "applied" && outcome.Verified && outcome.After != nil &&
			outcome.After.Verified && outcome.Evidence != "" {
			applied = true
		}
	}
	if !applied {
		t.Fatalf("no authoritative native add receipt for confirmed provider: %+v", result.Result.Outcomes)
	}
	newSource, err := os.ReadFile(sourcePath)
	if err != nil || bytes.Equal(newSource, oldSource) {
		t.Fatalf("real Edit did not persist changed portable intent: %v", err)
	}
	prefs, err := application.DesiredProviderPreferencesAt(stateRoot, desired)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, pref := range prefs {
		if pref.Requirement == (capability.Requirement{Kind: capability.SQL, Name: "default"}) &&
			pref.InstanceID == selected.ID && pref.ProviderKind == selected.Provider.Kind {
			found = true
		}
	}
	if !found {
		t.Fatalf("real Edit lost exact selected Target provider: %+v", prefs)
	}
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), []string{m.Name}, "plan")
	if err != nil {
		t.Fatal(err)
	}
	nativeRuntime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatalf("after-run native SQL runtime unavailable: %v", err)
	}
	runtimeFiles, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatalf("authoritative deployment runtime files unavailable after Edit: %v", err)
	}
	catalog, err := application.ObserveSQLConsumerInventory(ctx,
		application.NewRuntimeBackendProbeExecutor(nativeRuntime, runtimeFiles),
		runtimeFiles, resolved.Manifest, "default")
	if err != nil || !catalog.Empty ||
		catalog.Consumer != "application/"+m.ApplicationID+"/"+m.Environment {
		t.Fatalf("actual application SQL consumer/catalog not verified: %+v %v", catalog, err)
	}
	nativeFact := application.ProviderImpactFact{Realization: application.ProviderRealization{Owner: m.ApplicationID}}
	observeNativeBackendImpact(ctx, resolved, selected, &nativeFact)
	if !nativeFact.InventoryComplete {
		t.Fatalf("native post-edit consumer and material inventory incomplete: %+v", nativeFact)
	}
	hasOwnedVolume := false
	for _, material := range nativeFact.Materials {
		if material.Kind == "persistent-provider-data" && material.Verified && material.Location != nil {
			hasOwnedVolume = true
		}
	}
	if !hasOwnedVolume {
		t.Fatalf("native runtime did not observe its protected SQL data volume: %+v", nativeFact.Materials)
	}
	bindings, err := application.PreviewReferenceProviderRegistryAt(stateRoot, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range bindings.Bindings {
		if binding.Resource.Kind == capability.SQL && binding.Resource.Name == "default" &&
			binding.ProviderInstanceID == selected.ID {
			return
		}
	}
	t.Fatalf("real lifecycle did not preserve exact Target provider binding %q", selected.ID)
}

func TestAppEditCommandChecksGoSourceFormat(t *testing.T) {
	output, err := exec.Command("gofmt", "-l", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}
	if files := strings.TrimSpace(string(output)); files != "" {
		t.Fatalf("unformatted command sources:\n%s", files)
	}
}
