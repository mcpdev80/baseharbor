package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
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
	authorities.Execute = func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		return []application.ProviderExecutionOutcome{{InstanceID: "cache", Operation: "add", Status: "applied", Verified: true, Evidence: "cache add verified"}}, errors.New("old SQL stop failed")
	}
	result, err := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
	var partial *appMachineError
	if !errors.As(err, &partial) || partial.Code != "partial" {
		t.Fatalf("partial execution error = %v", err)
	}
	if executed != 1 || result.Result.Status != "partial" || !strings.Contains(result.Presentation, "old-sql: stop") || !strings.Contains(result.Presentation, "fresh consent") {
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
	if !errors.As(err, &machineErr) || machineErr.Code != "cancel" || executed != 0 {
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
