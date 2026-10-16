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
