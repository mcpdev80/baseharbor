package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
)

func TestManagedCoreLANServingTLSEndpoints(t *testing.T) {
	cfg := controlplaneruntime.Config{
		ListenAddr:          "192.168.6.178:8443",
		ConnectorListenAddr: "192.168.6.178:9443",
	}
	cfg.ConnectorEnrollmentEnabled = true
	t.Setenv("BASEHARBOR_API_PUBLIC_URL", "https://192.168.6.178:8443")
	t.Setenv("BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS", "192.168.6.178:9443")
	apiHost, connectorHost, err := managedCoreTLSEndpoints(cfg)
	if err != nil || apiHost != "192.168.6.178" || connectorHost != "192.168.6.178" {
		t.Fatalf("valid Core LAN identities: api=%q connector=%q err=%v", apiHost, connectorHost, err)
	}
	for _, tc := range []struct {
		name string
		cfg  controlplaneruntime.Config
		api  string
		node string
	}{
		{"loopback listener", controlplaneruntime.Config{ListenAddr: "127.0.0.1:8443", ConnectorListenAddr: cfg.ConnectorListenAddr}, "https://192.168.6.178:8443", "192.168.6.178:9443"},
		{"wildcard connector", controlplaneruntime.Config{ListenAddr: cfg.ListenAddr, ConnectorListenAddr: "0.0.0.0:9443"}, "https://192.168.6.178:8443", "192.168.6.178:9443"},
		{"API port mismatch", cfg, "https://192.168.6.178:8888", "192.168.6.178:9443"},
		{"connector port mismatch", cfg, "https://192.168.6.178:8443", "192.168.6.178:9999"},
		{"loopback public API", cfg, "https://127.0.0.1:8443", "192.168.6.178:9443"},
		{"loopback connector", cfg, "https://192.168.6.178:8443", "127.0.0.1:9443"},
		{"HTTP public API", cfg, "http://192.168.6.178:8443", "192.168.6.178:9443"},
		{"localhost pseudo-domain", cfg, "https://auth.baha.localhost:8443", "192.168.6.178:9443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(tc.name, "connector") {
				tc.cfg.ConnectorEnrollmentEnabled = true
			}
			t.Setenv("BASEHARBOR_API_PUBLIC_URL", tc.api)
			t.Setenv("BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS", tc.node)
			if _, _, err := managedCoreTLSEndpoints(tc.cfg); err == nil {
				t.Fatal("unsafe/mismatched serving endpoint admitted")
			}
		})
	}
}

func TestManagedCoreTLSNeverOverridesBYOCOrCoreAuthority(t *testing.T) {
	t.Setenv("BASEHARBOR_MANAGED_CORE_TLS", "true")
	for _, cfg := range []controlplaneruntime.Config{
		{},
		{ConnectorEnrollmentEnabled: true},
		{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: "core", RuntimeAppName: "demo"},
		{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: "core", TLSCertFile: "/etc/cert.pem"},
	} {
		if err := prepareManagedCoreServingTLS(t.Context(), &cfg); err == nil {
			t.Fatalf("managed serving TLS admitted invalid authority/config: %+v", cfg)
		}
	}
	t.Setenv("BASEHARBOR_MANAGED_CORE_TLS", "")
	cfg := controlplaneruntime.Config{TLSCertFile: "/etc/external.crt", TLSKeyFile: "/etc/external.key"}
	if err := prepareManagedCoreServingTLS(t.Context(), &cfg); err != nil || !strings.HasSuffix(cfg.TLSCertFile, ".crt") {
		t.Fatalf("default BYOC path changed: %+v %v", cfg, err)
	}
}

func TestManagedCoreLANEndpointDefaultsFromConcreteListener(t *testing.T) {
	t.Setenv("BASEHARBOR_API_PUBLIC_URL", "")
	t.Setenv("BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS", "")
	cfg := controlplaneruntime.Config{
		ListenAddr:                   "192.168.6.178:18443",
		WorkstationEnrollmentEnabled: true,
	}
	host, connector, err := managedCoreTLSEndpoints(cfg)
	if err != nil || host != "192.168.6.178" || connector != "" {
		t.Fatalf("automatic workstation-only LAN endpoint: host=%q connector=%q err=%v", host, connector, err)
	}
	for _, bad := range []string{"127.0.0.1:18443", "0.0.0.0:18443", "192.168.6.178:abc"} {
		cfg.ListenAddr = bad
		if _, _, err := managedCoreTLSEndpoints(cfg); err == nil {
			t.Fatalf("unsafe automatic Core LAN listener accepted: %q", bad)
		}
	}
}
