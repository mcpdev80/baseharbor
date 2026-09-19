package capability

import (
	"context"
	"testing"
)

type otlpTestDriver struct{ provider Provider }
func (d otlpTestDriver) Descriptor() Provider { return d.provider }
func (otlpTestDriver) Preflight(context.Context, Resource, Binding) error { return nil }
func (otlpTestDriver) Provision(context.Context, Resource, Binding) error { return nil }
func (otlpTestDriver) Bind(context.Context, Resource, Binding) error { return nil }
func (otlpTestDriver) Verify(context.Context, Resource, Binding) error { return nil }

func TestTelemetryOTLPSpecificationIsCanonical(t *testing.T) {
	spec, err := ParseSpecificationID("telemetry.otlp/v1")
	if err != nil { t.Fatal(err) }
	if spec != TelemetryOTLPV1 { t.Fatalf("spec = %#v", spec) }
}

func TestTelemetryOTLPBindingRequiresExplicitExportHTTPProtobuf(t *testing.T) {
	base := Request{
		Requirement: Requirement{Kind: TelemetryOTLP, Name: "default"},
		Workload: "application/demo",
		Driver: otlpTestDriver{provider: OTelCollector},
	}
	for name, binding := range map[string]*OTLPTelemetryBinding{
		"missing": nil,
		"receive": {Direction:"receive", Protocol:"http/protobuf", Signals:[]string{"traces"}},
		"grpc": {Direction:"export", Protocol:"grpc", Signals:[]string{"traces"}},
		"empty-signals": {Direction:"export", Protocol:"http/protobuf"},
		"unknown-signal": {Direction:"export", Protocol:"http/protobuf", Signals:[]string{"profiles"}},
	} {
		t.Run(name, func(t *testing.T) {
			req := base
			req.TelemetryOTLP = binding
			if _, err := BuildPlan("demo", []Request{req}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTelemetryOTLPBindingBuildsPortablePlan(t *testing.T) {
	req := Request{
		Requirement: Requirement{Kind: TelemetryOTLP, Name: "default"},
		Workload: "application/demo",
		TelemetryOTLP: &OTLPTelemetryBinding{Direction:"export", Protocol:"http/protobuf", Signals:[]string{"traces","metrics","logs"}},
		Driver: otlpTestDriver{provider: OTelCollector},
	}
	plan, err := BuildPlan("demo", []Request{req})
	if err != nil { t.Fatal(err) }
	if len(plan.Items) != 1 || plan.Items[0].Binding.TelemetryOTLP == nil {
		t.Fatalf("plan = %#v", plan)
	}
}
