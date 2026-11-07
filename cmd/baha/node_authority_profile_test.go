package main

import (
	"crypto/x509"
	"net"
	"testing"
)

func TestNodeAuthorityDefaultsAndCustom(t *testing.T) {
	profile := nodeAuthorityProfile{Environment: "dev", CoreURL: "https://core.example:8443", CoreAddress: "core.example:9443", CAFile: "/ca.pem"}
	defaults, err := applyNodeAuthorityDefaults(nodeAddOptions{NodeID: "edge", Runtime: "docker"}, profile)
	if err != nil || defaults.CoreURL != profile.CoreURL || defaults.CAFile != profile.CAFile || defaults.Environment != "dev" {
		t.Fatalf("node defaults were not applied: %+v, %v", defaults, err)
	}
	custom, err := applyNodeAuthorityDefaults(nodeAddOptions{Environment: "dev", CoreURL: "https://other.example", CAFile: "/other.pem"}, profile)
	if err != nil || custom.CoreURL != "https://other.example" || custom.CAFile != "/other.pem" {
		t.Fatalf("advanced node override was lost: %+v, %v", custom, err)
	}
	if _, err := applyNodeAuthorityDefaults(nodeAddOptions{Environment: "prod"}, profile); err == nil {
		t.Fatal("different environment admitted")
	}
}

func TestNodePublicEndpointFailClosed(t *testing.T) {
	cert := &x509.Certificate{DNSNames: []string{"core.example"}}
	host, err := discoverNodePublicHost("0.0.0.0", cert)
	if err != nil || host != "core.example" { t.Fatalf("single owned DNS SAN not selected: %q, %v", host, err) }
	if _, err := discoverNodePublicHost("127.0.0.1", &x509.Certificate{DNSNames: []string{"localhost"}}); err == nil {
		t.Fatal("loopback-only Core was accepted")
	}
	if _, err := discoverNodePublicHost("0.0.0.0", &x509.Certificate{DNSNames: []string{"a.example", "b.example"}}); err == nil {
		t.Fatal("ambiguous Core accepted")
	}
	ip := net.ParseIP("192.0.2.10")
	host, err = discoverNodePublicHost("0.0.0.0", &x509.Certificate{IPAddresses: []net.IP{ip}})
	if err != nil || host != ip.String() { t.Fatalf("single owned IP SAN not selected: %q, %v", host, err) }
}
