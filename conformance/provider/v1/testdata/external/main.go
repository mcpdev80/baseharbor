package main

import (
	"context"
	"errors"
	"fmt"
	provider "github.com/mcpdev80/baseharbor/conformance/provider/v1"
	"os"
)

type driver struct{ created, bound bool }

func (d *driver) ConformanceStateDigest() string {
	return fmt.Sprintf("created=%t;bound=%t", d.created, d.bound)
}
func (d *driver) Descriptor() provider.Provider {
	return provider.Provider{Kind: "external-proof", Capabilities: []provider.Kind{"metrics"}}
}
func (d *driver) Preflight(context.Context, provider.Resource, provider.Binding) error { return nil }
func (d *driver) Provision(context.Context, provider.Resource, provider.Binding) error {
	d.created = true
	return nil
}
func (d *driver) Bind(context.Context, provider.Resource, provider.Binding) error {
	if !d.created {
		return errors.New("resource missing")
	}
	d.bound = true
	return nil
}
func (d *driver) Verify(context.Context, provider.Resource, provider.Binding) error {
	if !d.bound {
		return errors.New("binding missing")
	}
	return nil
}

func main() {
	d := &driver{}
	descriptor := provider.IntegrationDescriptor{ID: "example/external-proof", Version: "1.0.0", Protocol: provider.ProtocolVersion,
		Provider: d.Descriptor(), Capabilities: []provider.SpecificationID{"metrics/v1"}, SupportedScopes: []provider.ProviderScope{provider.ScopeApplication},
		Observability: provider.ProviderObservability{Signals: []provider.ProviderObservabilitySignal{
			{Name: "metrics", Kind: "metrics", Status: "unsupported", Verification: "none"},
			{Name: "logs", Kind: "logs", Status: "unsupported", Verification: "none"},
			{Name: "traces", Kind: "traces", Status: "not-applicable", Verification: "none"},
		}}}
	target := provider.Target{Descriptor: descriptor, Application: "external-fixture", Request: provider.Request{Driver: d,
		Requirement: provider.Requirement{Kind: "metrics", Name: "application"}, Workload: "service/api", Metrics: &provider.MetricsBinding{Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics"}}}
	report := provider.Run(context.Background(), target)
	if err := provider.WriteJSON(os.Stdout, report); err != nil {
		os.Exit(2)
	}
	os.Exit(provider.ExitCode(report))
}
