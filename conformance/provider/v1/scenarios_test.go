package provider_test

import (
	"context"
	provider "github.com/mcpdev80/baseharbor/conformance/provider/v1"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
	"testing"
)

func fullFixture(driver provider.Driver) provider.Target {
	return provider.Target{Descriptor: capability.PrometheusIntegration, Application: "scenario-fixture", Request: provider.Request{Driver: driver, Requirement: provider.Requirement{Kind: "metrics", Name: "application"}, Workload: "service/api", Metrics: &provider.MetricsBinding{Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics"}}}
}
func TestPublicFullSuiteRetainsFailureRecoveryAndOwnershipProofs(t *testing.T) {
	driver := providerconformance.NewFakeDriver(capability.Prometheus)
	report := provider.RunFull(context.Background(), fullFixture(driver))
	if report.Status != provider.Pass {
		t.Fatalf("full report: %#v", report)
	}
	names := map[string]bool{}
	for _, c := range report.Checks {
		names[c.Name] = true
	}
	for _, required := range []string{"drift-repair-stable-identity", "outage-fails-without-mutation", "provision-retry", "malformed-binding-retry", "verify-retry", "foreign-ownership-blocks-before-mutation", "destroy-refuses-sibling", "destroy-owned-resource", "destroy-idempotent"} {
		if !names[required] {
			t.Fatalf("missing required check %s", required)
		}
	}
}

type unsafeDestroy struct {
	*providerconformance.FakeDriver
}

func (d unsafeDestroy) ConformanceDestroy(ctx context.Context, r provider.Resource) error { return nil }
func TestPublicFullSuiteRejectsUnprotectedDestroy(t *testing.T) {
	report := provider.RunFull(context.Background(), fullFixture(unsafeDestroy{providerconformance.NewFakeDriver(capability.Prometheus)}))
	if report.Status != provider.Fail {
		t.Fatal("unsafe destroy accepted")
	}
}
