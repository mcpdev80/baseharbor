package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/format"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func appMachineFixture() GuidedReadContext {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	return GuidedReadContext{
		ApplicationID: "existing-id", Target: "dev", Name: "demo", Environment: "dev",
		Capabilities: []capability.Requirement{req},
		Preferences:  []application.ProviderPreference{{Requirement: req, InstanceID: "db-dev"}},
		Footprint:    application.Footprint{Version: application.FootprintVersion, Target: "dev", Complete: true},
		PlanRevision: "impact-1", SnapshotRevision: "snapshot-1",
	}
}

func TestAppMachineAdaptersFootprintAndSelectionsParity(t *testing.T) {
	view := appMachineFixture()
	read := func(_ context.Context, target string) (GuidedReadContext, error) { return view, nil }
	got, err := appMachineInspect(context.Background(), "dev", read)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Requirements) != 1 || got.Requirements[0] != view.Capabilities[0] ||
		len(got.Preferences) != 1 || got.Preferences[0].InstanceID != "db-dev" {
		t.Fatalf("requirement/Target parity lost: %+v", got)
	}
	raw, err := appMachineFootprintJSON(view, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(view.Footprint)
	if !bytes.Equal(raw, want) {
		t.Fatalf("footprint differs: %s != %s", raw, want)
	}
	got.Preferences[0].InstanceID = "changed"
	if view.Preferences[0].InstanceID != "db-dev" {
		t.Fatal("reader state modified")
	}
}

func TestAppMachineAdaptersDeniedAndConflict(t *testing.T) {
	for _, kind := range []string{"denied", "wrong_target", "unsupported"} {
		_, err := appMachineInspect(context.Background(), "dev", func(context.Context, string) (GuidedReadContext, error) {
			return GuidedReadContext{}, &GuidedReadError{Kind: kind, Cause: errors.New("secret-token")}
		})
		var typed *appMachineError
		if !errors.As(err, &typed) || bytes.Contains([]byte(err.Error()), []byte("secret-token")) {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestAppMachineAdaptersStalePlanConsentAndCancellation(t *testing.T) {
	read := func(context.Context, string) (GuidedReadContext, error) { return appMachineFixture(), nil }
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", PlanID: "impact-1", ConfirmationsRequired: 2, Destructive: true}
	request := appMachineChangeRequest{Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1"}
	calls := 0
	planRead := func(context.Context, string) (application.ProviderImpactPlan, error) { return plan, nil }
	authorize := func(context.Context, string, string) error { return nil }
	apply := func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		calls++
		return nil, nil
	}
	cases := []struct {
		name   string
		mutate func(*appMachineChangeRequest)
		want   string
	}{
		{"stale-plan", func(r *appMachineChangeRequest) { r.PlanRevision = "old" }, "conflict"},
		{"stale-snapshot", func(r *appMachineChangeRequest) { r.SnapshotRevision = "old" }, "conflict"},
		{"stale-consent", func(r *appMachineChangeRequest) {
			r.Consent = application.ProviderChangeConsent{PlanID: "old", First: true, Second: true, DeleteData: true}
		}, "conflict"},
		{"cancel-first", func(r *appMachineChangeRequest) { r.Consent = application.ProviderChangeConsent{PlanID: "impact-1"} }, "cancel"},
		{"cancel-second", func(r *appMachineChangeRequest) {
			r.Consent = application.ProviderChangeConsent{PlanID: "impact-1", First: true}
		}, "cancel"},
		{"cancel-delete", func(r *appMachineChangeRequest) {
			r.Consent = application.ProviderChangeConsent{PlanID: "impact-1", First: true, Second: true}
		}, "cancel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := request
			tc.mutate(&req)
			_, err := appMachineChange(context.Background(), req, read, planRead, authorize, apply)
			var typed *appMachineError
			if !errors.As(err, &typed) || typed.Code != tc.want || calls != 0 {
				t.Fatalf("unexpected outcome: %v, calls=%d", err, calls)
			}
		})
	}
}

func TestAppMachineAdaptersPartialAndUnsupported(t *testing.T) {
	read := func(context.Context, string) (GuidedReadContext, error) { return appMachineFixture(), nil }
	plan := application.ProviderImpactPlan{
		Version: application.ProviderImpactVersion, Target: "dev",
		PlanID: "impact-1", ConfirmationsRequired: 2, Destructive: true,
		Impacts: []application.ProviderImpact{{InstanceID: "old", Operation: "retire"}},
	}
	req := appMachineChangeRequest{
		Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1",
		Consent: application.ProviderChangeConsent{PlanID: "impact-1", First: true, Second: true, DeleteData: true},
	}
	planRead := func(context.Context, string) (application.ProviderImpactPlan, error) { return plan, nil }
	authorize := func(context.Context, string, string) error { return nil }
	_, err := appMachineChange(context.Background(), req, read, planRead, authorize, nil)
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "unsupported" {
		t.Fatal(err)
	}
	result, err := appMachineChange(context.Background(), req, read, planRead, authorize,
		func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			return []application.ProviderExecutionOutcome{{InstanceID: "old", Operation: "retire", Status: "failed", ErrorCode: "provider-unavailable"}}, nil
		})
	if err != nil || result.Status != "failed" || len(result.Outcomes) != 1 {
		t.Fatalf("missing verified partial result: %+v %v", result, err)
	}
}

func TestAppMachineAdaptersHydrationParity(t *testing.T) {
	source := []byte("version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    instances:\n      primary: {}\n")
	view := appMachineFixture()
	before := bytes.Clone(source)
	loaded, err := appMachineHydrate(context.Background(), "dev", source, func(context.Context, string) (GuidedReadContext, error) {
		return view, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.Source, before) || len(loaded.Preferences) != 1 {
		t.Fatalf("hydration mismatch: %+v", loaded)
	}
}

func TestAppMachineAdaptersFormatting(t *testing.T) {
	for _, path := range []string{"app_machine_adapters.go", "app_machine_adapters_test.go"} {
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		formatted, err := format.Source(original)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, formatted) {
			t.Errorf("FORMATTED_%s=%q", path, string(formatted))
		}
	}
}

func TestAppMachineAdaptersAuthoritativePlanAndAuthorization(t *testing.T) {
	read := func(context.Context, string) (GuidedReadContext, error) { return appMachineFixture(), nil }
	req := appMachineChangeRequest{
		Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1",
		Consent: application.ProviderChangeConsent{PlanID: "impact-1", First: true},
	}
	applyCalls := 0
	apply := func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		applyCalls++
		return nil, nil
	}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", PlanID: "impact-1", ConfirmationsRequired: 1}
	planRead := func(context.Context, string) (application.ProviderImpactPlan, error) { return plan, nil }
	deny := func(context.Context, string, string) error { return errors.New("private authorization failure") }
	_, err := appMachineChange(context.Background(), req, read, planRead, deny, apply)
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "denied" || applyCalls != 0 {
		t.Fatalf("authorization did not fail closed: %v", err)
	}
	allow := func(context.Context, string, string) error { return nil }
	plan.PlanID = "stale"
	_, err = appMachineChange(context.Background(), req, read, planRead, allow, apply)
	if !errors.As(err, &typed) || typed.Code != "conflict" || applyCalls != 0 {
		t.Fatalf("stale authoritative plan applied: %v", err)
	}
	plan.PlanID = "impact-1"
	_, err = appMachineChange(context.Background(), req, read, nil, allow, apply)
	if !errors.As(err, &typed) || typed.Code != "unsupported" || applyCalls != 0 {
		t.Fatalf("missing plan owner accepted: %v", err)
	}
}

func TestAppMachineAdaptersManagedSecretsRequirements(t *testing.T) {
	view := appMachineFixture()
	secret := capability.Requirement{Kind: capability.Secrets, Name: "default"}
	view.Footprint.Requirements = []application.RequirementResolution{{Requirement: secret}}
	view.Preferences = append(view.Preferences, application.ProviderPreference{Requirement: secret})
	result, err := appMachineInspect(context.Background(), "dev",
		func(context.Context, string) (GuidedReadContext, error) { return view, nil })
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, requirement := range result.Requirements {
		if requirement == secret {
			count++
		}
	}
	if count != 1 || len(result.Preferences) != 2 {
		t.Fatalf("lost managed secrets or duplicated requirement: %+v", result)
	}
}

func TestAppMachineAdaptersDesiredNamedSQL(t *testing.T) {
	primary := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	analytics := capability.Requirement{Kind: capability.SQL, Name: "analytics"}
	view := appMachineFixture()
	read := func(context.Context, string) (GuidedReadContext, error) { return view, nil }
	want := appMachineDesiredRequest{Target: "dev", SnapshotRevision: "snapshot-1",
		Requirements: []capability.Requirement{primary, analytics},
		Choices:      []appMachineDesiredChoice{{Requirement: primary, InstanceID: "db-a"}, {Requirement: analytics, InstanceID: "db-b"}}}
	choiceRead := func(_ context.Context, target string, reqs []capability.Requirement) ([]appProviderChoice, error) {
		if target != "dev" || len(reqs) != 2 {
			t.Fatal("lost target/requirements")
		}
		return []appProviderChoice{
			{Target: target, Requirement: primary, Candidates: []appProviderCandidate{{Instance: capability.ProviderInstance{ID: "db-a"}}}},
			{Target: target, Requirement: analytics, Candidates: []appProviderCandidate{{Instance: capability.ProviderInstance{ID: "db-b"}}}},
		}, nil
	}
	planned := 0
	planner := func(_ context.Context, target string, reqs []capability.Requirement, prefs []application.ProviderPreference) (application.ProviderImpactPlan, error) {
		planned++
		if target != "dev" || len(reqs) != 2 || len(prefs) != 2 || prefs[0].Requirement != primary ||
			prefs[1].Requirement != analytics || prefs[0].InstanceID != "db-a" || prefs[1].InstanceID != "db-b" {
			t.Fatalf("desired selection lost before authority: %+v", prefs)
		}
		return application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target, PlanID: "fresh", Revision: "revision"}, nil
	}
	got, err := appMachinePropose(context.Background(), want, read, choiceRead, planner)
	if err != nil || planned != 1 || got.PlanRevision != "fresh" || len(got.Preferences) != 2 {
		t.Fatalf("authoritative result missing: %+v %v", got, err)
	}
	want.Choices[1].InstanceID = "wrong-kind"
	_, err = appMachinePropose(context.Background(), want, read, choiceRead, planner)
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "unsupported" || planned != 1 {
		t.Fatalf("incompatible choice reached planner: %v", err)
	}
	want.Target = "prod"
	_, err = appMachinePropose(context.Background(), want, read, choiceRead, planner)
	if !errors.As(err, &typed) || typed.Code != "conflict" {
		t.Fatalf("foreign target allowed: %v", err)
	}
}

func TestAppMachineAdaptersContextCancellation(t *testing.T) {
	for _, stage := range []string{"read", "authorize", "plan"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			applied := 0
			read := func(context.Context, string) (GuidedReadContext, error) {
				if stage == "read" {
					cancel()
					return GuidedReadContext{}, context.Canceled
				}
				return appMachineFixture(), nil
			}
			auth := func(context.Context, string, string) error {
				if stage == "authorize" {
					cancel()
					return context.Canceled
				}
				return nil
			}
			planRead := func(context.Context, string) (application.ProviderImpactPlan, error) {
				if stage == "plan" {
					cancel()
					return application.ProviderImpactPlan{}, context.Canceled
				}
				return application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", PlanID: "impact-1", ConfirmationsRequired: 1}, nil
			}
			_, err := appMachineChange(ctx, appMachineChangeRequest{
				Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1",
				Consent: application.ProviderChangeConsent{PlanID: "impact-1", First: true},
			}, read, planRead, auth, func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
				applied++
				return nil, nil
			})
			var typed *appMachineError
			if !errors.As(err, &typed) || typed.Code != "cancel" || applied != 0 {
				t.Fatalf("cancellation allowed execution: %v, apply=%d", err, applied)
			}
		})
	}
}

func TestAppMachineAdaptersPartialOnCancelledExecution(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", PlanID: "impact-1", ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{{InstanceID: "db", Operation: "retire"}}}
	got, err := appMachineChange(context.Background(), appMachineChangeRequest{
		Target: "dev", SnapshotRevision: "snapshot-1", PlanRevision: "impact-1",
		Consent: application.ProviderChangeConsent{PlanID: "impact-1", First: true},
	}, func(context.Context, string) (GuidedReadContext, error) { return appMachineFixture(), nil },
		func(context.Context, string) (application.ProviderImpactPlan, error) { return plan, nil },
		func(context.Context, string, string) error { return nil },
		func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			return []application.ProviderExecutionOutcome{{InstanceID: "db", Operation: "retire", Status: "failed", ErrorCode: "interrupted"}}, context.Canceled
		})
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "partial" || len(got.Outcomes) != 1 || got.Outcomes[0].ErrorCode != "interrupted" {
		t.Fatalf("execution evidence lost: %+v, %v", got, err)
	}
}

func TestAppMachineAdaptersProposeConsentApplyEndToEnd(t *testing.T) {
	primary := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	analytics := capability.Requirement{Kind: capability.SQL, Name: "analytics"}
	desired := appMachineDesiredRequest{
		Target: "dev", SnapshotRevision: "snapshot-1",
		Requirements: []capability.Requirement{primary, analytics},
		Choices: []appMachineDesiredChoice{{Requirement: primary, InstanceID: "db-a"}, {Requirement: analytics, InstanceID: "db-b"}},
	}
	current := appMachineFixture() // current PlanRevision is impact-1, NOT change-42
	read := func(context.Context, string) (GuidedReadContext, error) { return current, nil }
	choices := func(_ context.Context, target string, reqs []capability.Requirement) ([]appProviderChoice, error) {
		return []appProviderChoice{
			{Target: target, Requirement: primary, Candidates: []appProviderCandidate{{Instance: capability.ProviderInstance{ID: "db-a"}}}},
			{Target: target, Requirement: analytics, Candidates: []appProviderCandidate{{Instance: capability.ProviderInstance{ID: "db-b"}}}},
		}, nil
	}
	planCalls := 0
	planner := func(_ context.Context, target string, reqs []capability.Requirement, prefs []application.ProviderPreference) (application.ProviderImpactPlan, error) {
		planCalls++
		if len(reqs) != 2 || len(prefs) != 2 || prefs[0].Requirement != primary ||
			prefs[1].Requirement != analytics || prefs[0].InstanceID != "db-a" || prefs[1].InstanceID != "db-b" {
			t.Fatalf("authority lost typed intent: %+v %+v", reqs, prefs)
		}
		return application.ProviderImpactPlan{
			Version: application.ProviderImpactVersion, Target: target, PlanID: "change-42",
			Revision: "change-rev-42", ConfirmationsRequired: 2, Destructive: true,
			Impacts: []application.ProviderImpact{{InstanceID: "db-a", Operation: "provide"}, {InstanceID: "db-b", Operation: "provide"}},
		}, nil
	}
	proposal, err := appMachinePropose(context.Background(), desired, read, choices, planner)
	if err != nil || proposal.PlanRevision != "change-42" || proposal.PlanRevision == current.PlanRevision {
		t.Fatalf("not a fresh change plan: %+v %v", proposal, err)
	}
	req := appMachineDesiredChangeRequest{
		Desired: desired, Proposal: proposal,
		Consent: application.ProviderChangeConsent{PlanID: proposal.Plan.PlanID, First: true, Second: true, DeleteData: true},
	}
	applied := 0
	authorize := func(context.Context, string, string) error { return nil }
	apply := func(_ context.Context, plan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		applied++
		if plan.PlanID != "change-42" || consent.PlanID != plan.PlanID {
			t.Fatalf("wrong applied plan %s", plan.PlanID)
		}
		return []application.ProviderExecutionOutcome{
			{InstanceID: "db-a", Operation: "provide", Status: "applied"},
			{InstanceID: "db-b", Operation: "provide", Status: "applied"},
		}, nil
	}
	result, err := appMachineChangeDesired(context.Background(), req, read, choices, planner, authorize, apply)
	if err != nil || applied != 1 || len(result.Outcomes) != 2 || planCalls != 2 {
		t.Fatalf("propose -> consent -> apply failed: %+v err=%v calls=%d planned=%d", result, err, applied, planCalls)
	}
	checkRejected := func(label string, request appMachineDesiredChangeRequest, reader appMachineRead) {
		t.Helper()
		before := applied
		_, err := appMachineChangeDesired(context.Background(), request, reader, choices, planner, authorize, apply)
		var machineErr *appMachineError
		if !errors.As(err, &machineErr) || machineErr.Code != "conflict" || applied != before {
			t.Fatalf("%s: unsafe execution: %v applied=%d", label, err, applied)
		}
	}
	stale := current
	stale.SnapshotRevision = "snapshot-2"
	checkRejected("stale snapshot", req, func(context.Context, string) (GuidedReadContext, error) { return stale, nil })
	changed := req
	changed.Desired.Choices = append([]appMachineDesiredChoice(nil), req.Desired.Choices...)
	changed.Desired.Choices[1].InstanceID = "db-a"
	checkRejected("changed intent", changed, read)
	badConsent := req
	badConsent.Consent.PlanID = current.PlanRevision
	checkRejected("wrong consent", badConsent, read)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := applied
	_, err = appMachineChangeDesired(ctx, req, read, choices, planner, authorize, apply)
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "cancel" || applied != before {
		t.Fatalf("cancel allowed execution: %v", err)
	}
	partialApply := func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		return []application.ProviderExecutionOutcome{{InstanceID: "db-a", Operation: "provide", Status: "applied"}, {InstanceID: "db-b", Operation: "provide", Status: "failed", ErrorCode: "interrupted"}}, context.Canceled
	}
	partial, err := appMachineChangeDesired(context.Background(), req, read, choices, planner, authorize, partialApply)
	if !errors.As(err, &typed) || typed.Code != "partial" || len(partial.Outcomes) != 2 {
		t.Fatalf("lost partial outcomes: %+v %v", partial, err)
	}
}
