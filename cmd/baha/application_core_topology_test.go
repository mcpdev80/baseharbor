package main

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"testing"
)

func TestApplicationSecretsHARequiresActualBoundCoreTopology(t *testing.T) {
	m := application.WithHA(application.New("topology", "dev", true, false, true), true)
	if err := requireApplicationSecretsTopology(m, false); err == nil {
		t.Fatal("bound Single Core silently satisfied requested secrets HA")
	}
	if err := requireApplicationSecretsTopology(m, true); err != nil {
		t.Fatal(err)
	}
	no := false
	m.Availability = map[string]availability.Override{"secrets": {HA: &no}}
	if err := requireApplicationSecretsTopology(m, false); err != nil {
		t.Fatal("component override ignored")
	}
	m.Availability = nil
	m.Services.Secrets = false
	if err := requireApplicationSecretsTopology(m, false); err != nil {
		t.Fatal("PKI dependency implicitly required secrets HA")
	}
}
