package devgateway

import "testing"

func TestConfiguredDomainUsesOwnedCoreLANAndPreservesBindOverrides(t *testing.T) {
	t.Setenv("BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS", "")
	routes := []Route{{Host: "app.mcp-dev.de"}, {Host: "sql.mcp-dev.de"}}
	got, err := gatewayBindAddressForIPAccess(routes, "192.168.50.5")
	if err != nil || got != "192.168.50.5" {
		t.Fatalf("custom domain not reachable on selected Core: %s %v", got, err)
	}
	got, err = gatewayBindAddressForIPAccess(routes, "")
	if err != nil || got != "127.0.0.1" {
		t.Fatal("missing owned IP invented LAN listener")
	}
	local := append(append([]Route(nil), routes...), Route{Host: "other.baha.localhost"})
	got, err = gatewayBindAddressForIPAccess(local, "192.168.50.5")
	if err != nil || got != "127.0.0.1" {
		t.Fatal("localhost routes exposed on LAN")
	}
	t.Setenv("BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS", "127.0.0.1")
	got, err = gatewayBindAddressForIPAccess(routes, "192.168.50.5")
	if err != nil || got != "127.0.0.1" {
		t.Fatal("explicit loopback override ignored")
	}
	t.Setenv("BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS", "0.0.0.0")
	if _, err = gatewayBindAddressForIPAccess(routes, "192.168.50.5"); err == nil {
		t.Fatal("wildcard override accepted")
	}
}
