package main

import (
	"context"
	"errors"
	"fmt"
	provider "github.com/mcpdev80/baseharbor/conformance/provider/v1"
	"os"
)

type driver struct {
	created, bound, drifted bool
	owner                   string
	failure                 provider.FailureMode
	ownership               provider.Ownership
}

func (d *driver) SetFailure(mode provider.FailureMode)                { d.failure = mode }
func (d *driver) SetReconciliationOwnership(owner provider.Ownership) { d.ownership = owner }
func (d *driver) ConformanceDrift()                                   { d.drifted = true }
func (d *driver) DesiredState(r provider.Resource, b provider.Binding) provider.Desired {
	return provider.Desired{Exists: true, Digest: r.Application + "/" + r.Name, Owner: provider.OwnershipBaseHarbor}
}
func (d *driver) Observe(ctx context.Context, r provider.Resource, b provider.Binding) (provider.Observed, error) {
	digest := r.Application + "/" + r.Name
	if d.drifted {
		digest = "fixture-drift"
	}
	return provider.Observed{Exists: d.created, Digest: digest, Owner: d.ownership}, nil
}
func (d *driver) ConformanceDestroy(ctx context.Context, r provider.Resource) error {
	if !d.created {
		return nil
	}
	if r.Application != d.owner {
		return errors.New("foreign ownership")
	}
	d.created, d.bound, d.drifted = false, false, false
	return nil
}

func (d *driver) ConformanceStateDigest() string {
	return fmt.Sprintf("created=%t;bound=%t;drifted=%t;owner=%s", d.created, d.bound, d.drifted, d.owner)
}
func (d *driver) Descriptor() provider.Provider {
	return provider.Provider{Kind: "external-proof", Capabilities: []provider.Kind{"metrics"}}
}
func (d *driver) Preflight(context.Context, provider.Resource, provider.Binding) error {
	if d.failure == provider.FailureUnavailable {
		return errors.New("fixture outage")
	}
	return nil
}
func (d *driver) Provision(ctx context.Context, r provider.Resource, b provider.Binding) error {
	if d.failure == provider.FailureProvision {
		return errors.New("fixture provision failure")
	}
	d.owner = r.Application
	d.drifted = false
	d.created = true
	return nil
}
func (d *driver) Bind(context.Context, provider.Resource, provider.Binding) error {
	if d.failure == provider.FailureMalformedBinding {
		return errors.New("fixture malformed binding")
	}
	if !d.created {
		return errors.New("resource missing")
	}
	d.bound = true
	return nil
}
func (d *driver) Verify(context.Context, provider.Resource, provider.Binding) error {
	if d.failure == provider.FailureVerify {
		return errors.New("fixture verify failure")
	}
	if !d.bound || d.drifted {
		return errors.New("binding missing")
	}
	return nil
}

func main() {
	d := &driver{ownership: provider.OwnershipBaseHarbor}
	descriptor := provider.IntegrationDescriptor{ID: "example/external-proof", Version: "1.0.0", Protocol: provider.ProtocolVersion,
		Provider: d.Descriptor(), Capabilities: []provider.SpecificationID{"metrics/v1"}, SupportedScopes: []provider.ProviderScope{provider.ScopeApplication},
		Observability: provider.ProviderObservability{Signals: []provider.ProviderObservabilitySignal{
			{Name: "metrics", Kind: "metrics", Status: "unsupported", Verification: "none"},
			{Name: "logs", Kind: "logs", Status: "unsupported", Verification: "none"},
			{Name: "traces", Kind: "traces", Status: "not-applicable", Verification: "none"},
		}}}
	target := provider.Target{Descriptor: descriptor, Application: "external-fixture", Request: provider.Request{Driver: d,
		Requirement: provider.Requirement{Kind: "metrics", Name: "application"}, Workload: "service/api", Metrics: &provider.MetricsBinding{Direction: "provide", Format: "openmetrics", Service: "api", Port: 8080, Path: "/metrics"}}}
	report := provider.RunFull(context.Background(), target)
	if err := provider.WriteJSON(os.Stdout, report); err != nil {
		os.Exit(2)
	}
	os.Exit(provider.ExitCode(report))
}
