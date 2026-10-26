package provider_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	provider "github.com/mcpdev80/baseharbor/conformance/provider/v1"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
)

type leakingFixtureDriver struct {
	*providerconformance.FakeDriver
}

func (d leakingFixtureDriver) Verify(context.Context, provider.Resource, provider.Binding) error {
	return errors.New("provider password=DO_NOT_LEAK")
}

func TestPublicConformanceReportDoesNotEchoProviderSecrets(t *testing.T) {
	driver := leakingFixtureDriver{providerconformance.NewFakeDriver(capability.Prometheus)}
	target := provider.Target{Descriptor: capability.PrometheusIntegration, Application: "conformance-fixture", Request: provider.Request{
		Driver: driver, Requirement: provider.Requirement{Kind: "metrics", Name: "application"}, Workload: "service/api",
		Metrics: &provider.MetricsBinding{Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics"}}}
	report := provider.Run(context.Background(), target)
	var output bytes.Buffer
	if err := provider.WriteJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	if report.Status != provider.Fail || bytes.Contains(output.Bytes(), []byte("DO_NOT_LEAK")) {
		t.Fatalf("unsafe public report: %s", output.Bytes())
	}
}

func TestVersionedProfileRetainsExistingHarness(t *testing.T) {
	target := provider.Target{Descriptor: capability.PrometheusIntegration, Application: "conformance-fixture",
		Request: provider.Request{Driver: providerconformance.NewFakeDriver(capability.Prometheus),
			Requirement: provider.Requirement{Kind: "metrics", Name: "application"}, Workload: "service/api",
			Metrics: &provider.MetricsBinding{Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics"}}}
	report := provider.Run(context.Background(), target)
	if provider.ExitCode(report) != 0 || report.Profile != provider.ProfileID || report.SchemaVersion != provider.ReportVersion {
		t.Fatalf("report = %#v", report)
	}
	var first, second bytes.Buffer
	if err := provider.WriteJSON(&first, report); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteJSON(&second, report); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("report is nondeterministic")
	}
	if bytes.Contains(first.Bytes(), []byte("fake-secret")) {
		t.Fatal("report leaked provider credentials")
	}
	target.Request.Driver = nil
	if got := provider.Run(context.Background(), target); provider.ExitCode(got) != 1 {
		t.Fatal("missing driver accepted")
	}
}
