package capability

import (
	"context"
	"errors"
	"testing"
)

type testDriver struct {
	provider       Provider
	preflightErr   error
	provisionErr   error
	bindErr        error
	verifyErr      error
	preflightCalls int
	provisionCalls int
	bindCalls      int
	verifyCalls    int
}

func (d *testDriver) Descriptor() Provider { return d.provider }
func (d *testDriver) Preflight(context.Context, Resource, Binding) error {
	d.preflightCalls++
	return d.preflightErr
}
func (d *testDriver) Provision(context.Context, Resource) error {
	d.provisionCalls++
	return d.provisionErr
}
func (d *testDriver) Bind(context.Context, Resource, Binding) error {
	d.bindCalls++
	return d.bindErr
}
func (d *testDriver) Verify(context.Context, Resource, Binding) error {
	d.verifyCalls++
	return d.verifyErr
}

func TestRunCompletesResolvePreflightApplyBindVerify(t *testing.T) {
	driver := &testDriver{provider: PostgreSQL}
	result, err := Run(context.Background(), "mailflow", []Request{{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/mailflow",
		Driver:      driver,
	}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusReady {
		t.Fatalf("Run() status = %q, want ready", result.Status)
	}
	if len(result.Plan.Items) != 1 || len(result.Steps) != 4 {
		t.Fatalf("Run() result = %#v", result)
	}
	if driver.preflightCalls != 1 || driver.provisionCalls != 1 || driver.bindCalls != 1 || driver.verifyCalls != 1 {
		t.Fatalf("driver calls = preflight:%d provision:%d bind:%d verify:%d", driver.preflightCalls, driver.provisionCalls, driver.bindCalls, driver.verifyCalls)
	}
}

func TestRunPreflightsAllResourcesBeforeMutation(t *testing.T) {
	first := &testDriver{provider: PostgreSQL}
	second := &testDriver{provider: Valkey, preflightErr: errors.New("unsupported deployment")}
	result, err := Run(context.Background(), "mailflow", []Request{
		{Requirement: Requirement{Kind: SQL, Name: "primary"}, Workload: "application/mailflow", Driver: first},
		{Requirement: Requirement{Kind: KeyValue, Name: "cache"}, Workload: "application/mailflow", Driver: second},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want preflight failure")
	}
	if result.Status != StatusFailed {
		t.Fatalf("Run() status = %q, want failed", result.Status)
	}
	if first.preflightCalls != 1 || second.preflightCalls != 1 {
		t.Fatalf("preflight calls = %d/%d, want 1/1", first.preflightCalls, second.preflightCalls)
	}
	if first.provisionCalls != 0 || second.provisionCalls != 0 {
		t.Fatalf("mutation occurred before all preflights passed: %d/%d", first.provisionCalls, second.provisionCalls)
	}
}

func TestBuildPlanFailsUnsupportedProviderBeforeMutation(t *testing.T) {
	driver := &testDriver{provider: Valkey}
	_, err := Run(context.Background(), "mailflow", []Request{{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/mailflow",
		Driver:      driver,
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want negotiation failure")
	}
	if driver.preflightCalls != 0 || driver.provisionCalls != 0 || driver.bindCalls != 0 || driver.verifyCalls != 0 {
		t.Fatal("unsupported provider reached lifecycle hooks")
	}
}

func TestRunStopsAfterVerificationFailure(t *testing.T) {
	driver := &testDriver{provider: PostgreSQL, verifyErr: errors.New("SELECT 1 failed")}
	result, err := Run(context.Background(), "mailflow", []Request{{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/mailflow",
		Driver:      driver,
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want verification failure")
	}
	if result.Status != StatusFailed {
		t.Fatalf("Run() status = %q, want failed", result.Status)
	}
	last := result.Steps[len(result.Steps)-1]
	if last.Phase != PhaseVerify || last.Status != StatusFailed || len(last.Diagnostics) != 1 {
		t.Fatalf("verification result = %#v", last)
	}
}
