package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func appMachineFixture() GuidedReadContext {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	return GuidedReadContext{
		ApplicationID: "existing-id", Target: "dev", Name: "demo", Environment: "dev",
		Capabilities:	[]capability.Requirement{req},
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
		{"stale-consent", func(r *appMachineChangeRequest) { r.Consent = application.ProviderChangeConsent{PlanID: "old", First: true, Second: true, DeleteData: true} }, "conflict"},
		{"cancel-first", func(r *appMachineChangeRequest) { r.Consent = application.ProviderChangeConsent{PlanID: "impact-1"} }, "cancel"},
		{"cancel-second", func(r *appMachineChangeRequest) { r.Consent = application.ProviderChangeConsent{PlanID: "impact-1", First: true} }, "cancel"},
		{"cancel-delete", func(r *appMachineChangeRequest) { r.Consent = application.ProviderChangeConsent{PlanID: "impact-1", First: true, Second: true} }, "cancel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := request
			tc.mutate(&req)
			_, err := appMachineChange(context.Background(), req, read, plan, apply)
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
	_, err := appMachineChange(context.Background(), req, read, plan, nil)
	var typed *appMachineError
	if !errors.As(err, &typed) || typed.Code != "unsupported" {
		t.Fatal(err)
	}
	result, err := appMachineChange(context.Background(), req, read, plan,
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
