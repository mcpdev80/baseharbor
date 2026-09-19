package capability

import (
	"context"
	"errors"
	"strings"
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
func (d *testDriver) Provision(context.Context, Resource, Binding) error {
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
	if len(result.Plan.Items) != 1 || len(result.Steps) != 5 {
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

func TestPrepareCompletesAllPreflightBeforeMutation(t *testing.T) {
	var calls []string
	first := &recordingDriver{provider: Provider{Kind: ProviderPostgreSQL, Capabilities: []Kind{SQL}}, calls: &calls}
	second := &recordingDriver{provider: Provider{Kind: ProviderValkey, Capabilities: []Kind{KeyValue}}, calls: &calls}
	execution, result, err := Prepare(context.Background(), "mailflow", []Request{
		{Requirement: Requirement{Kind: SQL, Name: "primary"}, Workload: "application/mailflow", Driver: first},
		{Requirement: Requirement{Kind: KeyValue, Name: "cache"}, Workload: "application/mailflow", Driver: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if execution == nil || result.Status != StatusReady {
		t.Fatalf("unexpected prepare result: execution=%v result=%#v", execution, result)
	}
	if got := strings.Join(calls, ","); got != "preflight:postgresql,preflight:valkey" {
		t.Fatalf("Prepare calls = %q", got)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "preflight:postgresql,preflight:valkey,provision:postgresql,bind:postgresql,provision:valkey,bind:valkey" {
		t.Fatalf("ProvisionAndBind calls = %q", got)
	}
	if _, err := execution.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareFailureDoesNotMutateAnyProvider(t *testing.T) {
	var calls []string
	first := &recordingDriver{provider: Provider{Kind: ProviderPostgreSQL, Capabilities: []Kind{SQL}}, calls: &calls}
	second := &recordingDriver{provider: Provider{Kind: ProviderValkey, Capabilities: []Kind{KeyValue}}, calls: &calls, preflightErr: errors.New("blocked")}
	execution, _, err := Prepare(context.Background(), "mailflow", []Request{
		{Requirement: Requirement{Kind: SQL, Name: "primary"}, Workload: "application/mailflow", Driver: first},
		{Requirement: Requirement{Kind: KeyValue, Name: "cache"}, Workload: "application/mailflow", Driver: second},
	})
	if err == nil || execution != nil {
		t.Fatalf("expected fail-closed prepare, execution=%v err=%v", execution, err)
	}
	if got := strings.Join(calls, ","); got != "preflight:postgresql,preflight:valkey" {
		t.Fatalf("preflight failure mutated provider: calls=%q", got)
	}
}

type recordingDriver struct {
	provider     Provider
	calls        *[]string
	preflightErr error
}

func (d *recordingDriver) Descriptor() Provider { return d.provider }
func (d *recordingDriver) Preflight(context.Context, Resource, Binding) error {
	*d.calls = append(*d.calls, "preflight:"+string(d.provider.Kind))
	return d.preflightErr
}
func (d *recordingDriver) Provision(context.Context, Resource, Binding) error {
	*d.calls = append(*d.calls, "provision:"+string(d.provider.Kind))
	return nil
}
func (d *recordingDriver) Bind(context.Context, Resource, Binding) error {
	*d.calls = append(*d.calls, "bind:"+string(d.provider.Kind))
	return nil
}
func (d *recordingDriver) Verify(context.Context, Resource, Binding) error {
	*d.calls = append(*d.calls, "verify:"+string(d.provider.Kind))
	return nil
}


func TestBuildPlanCarriesValidatedSecureBinding(t *testing.T) {
	driver := &testDriver{provider: PostgreSQL}
	security := SecureBinding{
		Identity: &WorkloadIdentityBinding{
			Subject:   "spiffe://baseharbor/apps/mailflow/production",
			Reference: "baseharbor://applications/mailflow/production/identities/runtime",
		},
		Credentials: []CredentialReference{{
			Name:      "database",
			Reference: "baseharbor://applications/mailflow/production/credentials/database",
		}},
	}
	plan, err := BuildPlan("mailflow", []Request{{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/mailflow",
		Security:    &security,
		Driver:      driver,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Binding.Security == nil {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Items[0].Binding.Security.Identity == nil ||
		plan.Items[0].Binding.Security.Identity.Subject != "spiffe://baseharbor/apps/mailflow/production" {
		t.Fatalf("secure binding = %#v", plan.Items[0].Binding.Security)
	}
}

func TestBuildPlanRejectsInvalidSecureBindingBeforeProviderPreflight(t *testing.T) {
	driver := &testDriver{provider: PostgreSQL}
	security := SecureBinding{
		Credentials: []CredentialReference{{Name: "database"}},
	}
	_, err := Run(context.Background(), "mailflow", []Request{{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/mailflow",
		Security:    &security,
		Driver:      driver,
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want secure-binding validation failure")
	}
	if driver.preflightCalls != 0 || driver.provisionCalls != 0 || driver.bindCalls != 0 || driver.verifyCalls != 0 {
		t.Fatal("invalid secure binding reached provider lifecycle")
	}
}
