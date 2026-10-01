package provider_test

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/sdk/provider"
)

type externalDriver struct {
	provider provider.Provider
}

func (d *externalDriver) Descriptor() provider.Provider { return d.provider }
func (d *externalDriver) Preflight(context.Context, provider.Resource, provider.Binding) error { return nil }
func (d *externalDriver) Provision(context.Context, provider.Resource, provider.Binding) error { return nil }
func (d *externalDriver) Bind(context.Context, provider.Resource, provider.Binding) error { return nil }
func (d *externalDriver) Verify(context.Context, provider.Resource, provider.Binding) error { return nil }

func TestExternalPackageCanImplementDriver(t *testing.T) {
	driver := &externalDriver{provider: provider.Provider{
		Kind: "example-postgresql",
		Capabilities: []provider.Kind{provider.SQL},
	}}
	var _ provider.Driver = driver
	plan, err := provider.BuildPlan("demo", []provider.Request{{
		Requirement: provider.Requirement{Kind: provider.SQL, Name: "default"},
		Workload: "app",
		Driver: driver,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("plan items = %d", len(plan.Items))
	}
}

func TestSDKExportsAllNewV0419CapabilityKinds(t *testing.T) {
	for _, kind := range []provider.Kind{
		provider.DurableKeyValue,
		provider.DocumentDatabase,
		provider.MessagingQueue,
		provider.MessagingPubSub,
		provider.MessagingStream,
	} {
		driver := &externalDriver{provider: provider.Provider{
			Kind: "example-provider",
			Capabilities: []provider.Kind{kind},
		}}
		plan, err := provider.BuildPlan("demo", []provider.Request{{
			Requirement: provider.Requirement{Kind: kind, Name: "default"},
			Workload:    "app",
			Driver:      driver,
		}})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(plan.Items) != 1 || plan.Items[0].Resource.Kind != kind {
			t.Fatalf("%s plan = %#v", kind, plan)
		}
	}
}
