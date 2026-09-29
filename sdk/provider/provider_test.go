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
