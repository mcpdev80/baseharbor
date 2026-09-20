package providerconformance

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func fakeTarget(driver *FakeDriver) Target {
	resource := capability.Requirement{Kind: capability.Metrics, Name: "application"}
	return Target{
		Descriptor: capability.PrometheusIntegration,
		Application: "demo",
		UnsupportedScope: capability.ScopeExternal,
		Request: capability.Request{
			Requirement: resource,
			Workload: "service/api",
			Metrics: &capability.MetricsBinding{
				Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics",
			},
			Driver: driver,
		},
	}
}

func TestExecutableProviderConformanceLifecycle(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	report := Run(context.Background(), fakeTarget(driver))
	if err := Require(report); err != nil {
		t.Fatalf("conformance: %v (%#v)", err, report)
	}
	stats := driver.Stats()
	if stats.Creates != 1 || stats.Noops != 1 {
		t.Fatalf("expected CREATE then NOOP, got %#v", stats)
	}
}

func TestFakeProviderDriftRepairAndStableIdentity(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	target := fakeTarget(driver)
	if err := Require(Run(context.Background(), target)); err != nil {
		t.Fatal(err)
	}
	before := driver.ConformanceStateDigest()
	driver.ConformanceDrift()

	execution, _, err := capability.Prepare(context.Background(), "demo", []capability.Request{target.Request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := driver.ConformanceStateDigest()
	if before != after {
		t.Fatalf("repair changed stable provider identity/state: before=%s after=%s", before, after)
	}
	if driver.Stats().Repairs != 1 {
		t.Fatalf("repair stats=%#v", driver.Stats())
	}
}

func TestFakeProviderFailureBetweenLifecyclePhasesRecovers(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	target := fakeTarget(driver)

	driver.SetFailure(FailureMalformedBinding)
	execution, _, err := capability.Prepare(context.Background(), "demo", []capability.Request{target.Request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err == nil {
		t.Fatal("malformed binding did not fail closed")
	}
	driver.SetFailure(FailureNone)

	execution, _, err = capability.Prepare(context.Background(), "demo", []capability.Request{target.Request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := execution.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if driver.Stats().Creates != 1 {
		t.Fatalf("retry recreated provider resource: %#v", driver.Stats())
	}
}

func TestFakeProviderVerifyFailureNeverReportsReadyAndRetryConverges(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	target := fakeTarget(driver)
	driver.SetFailure(FailureVerify)

	execution, _, err := capability.Prepare(context.Background(), "demo", []capability.Request{target.Request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := execution.Verify(context.Background())
	if err == nil || result.Status != capability.StatusFailed {
		t.Fatalf("verify failure reported ready: status=%s err=%v", result.Status, err)
	}
	if !driver.DiagnosticSafe(err) {
		t.Fatal("secret leaked through provider diagnostics")
	}

	driver.SetFailure(FailureNone)
	execution, _, err = capability.Prepare(context.Background(), "demo", []capability.Request{target.Request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execution.ProvisionAndBind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result, err = execution.Verify(context.Background()); err != nil || result.Status != capability.StatusReady {
		t.Fatalf("retry did not converge: status=%s err=%v", result.Status, err)
	}
}

func TestFakeProviderDestroyEnforcesOwnership(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	target := fakeTarget(driver)
	if err := Require(Run(context.Background(), target)); err != nil {
		t.Fatal(err)
	}
	resource := capability.Resource{Application: "demo", Kind: capability.Metrics, Name: "application", Provider: capability.ProviderPrometheus}
	other := resource
	other.Application = "other"
	if err := driver.ConformanceDestroy(context.Background(), other); err == nil {
		t.Fatal("destroy accepted another application's resource")
	}
	if err := driver.ConformanceDestroy(context.Background(), resource); err != nil {
		t.Fatal(err)
	}
	if driver.Stats().Destroys != 1 {
		t.Fatalf("destroy stats=%#v", driver.Stats())
	}
}

func TestFakeProviderPreflightFailureHasNoMutation(t *testing.T) {
	driver := NewFakeDriver(capability.Prometheus)
	target := fakeTarget(driver)
	before := driver.ConformanceStateDigest()
	driver.SetFailure(FailureUnavailable)
	if report := Run(context.Background(), target); report.Status != Fail {
		t.Fatalf("provider outage unexpectedly passed: %#v", report)
	}
	if after := driver.ConformanceStateDigest(); after != before {
		t.Fatalf("preflight failure mutated provider: before=%s after=%s", before, after)
	}
}
