package serviceaccess

import (
	"strings"
	"testing"
)

func TestCaddyfileRequiresClientCertificateWhenRequested(t *testing.T) {
	got := caddyfile("http://prometheus:9090", 8443, AuthenticationMTLS)
	for _, want := range []string{"tls /certs/server.pem /certs/server-key.pem", "mode require_and_verify", "reverse_proxy http://prometheus:9090"} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestGatewayComposePublishesOnlyTLSPort(t *testing.T) {
	files := HTTPGatewayFiles{
		Caddyfile: "/tmp/access/Caddyfile",
		Material: TLSMaterial{
			CA:                "/tmp/access/ca.pem",
			ServerCertificate: "/tmp/access/server.pem",
			ServerKey:         "/tmp/access/server-key.pem",
		},
	}
	got := HTTPGatewayComposeService(files, HTTPGatewaySpec{
		ServiceName:      "prometheus-access",
		PublishedPortEnv: "BASEHARBOR_PROMETHEUS_PORT",
		ContainerPort:    8443,
		Networks:         []string{"provider"},
	})
	if !strings.Contains(got, "127.0.0.1:$"+"{BASEHARBOR_PROMETHEUS_PORT}:8443") {
		t.Fatalf("gateway does not publish TLS loopback port:\n%s", got)
	}
	if strings.Contains(got, ":9090") {
		t.Fatalf("gateway unexpectedly publishes upstream clear-text port:\n%s", got)
	}
}

func TestCaddyfileRequiresBearerTokenWhenSelected(t *testing.T) {
	got := caddyfile("http://prometheus:9090", 8443, AuthenticationToken)
	for _, want := range []string{
		"Bearer {$BASEHARBOR_ACCESS_TOKEN}",
		"respond @unauthorized 401",
		"reverse_proxy http://prometheus:9090",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("token gateway config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "client_auth") {
		t.Fatalf("token gateway unexpectedly requires mTLS:\n%s", got)
	}
}

func TestGatewayComposeRunsCaddyWithLeastPrivilege(t *testing.T) {
	files := HTTPGatewayFiles{
		Caddyfile: "/tmp/access/Caddyfile",
		Material: TLSMaterial{
			CA:                "/tmp/access/ca.pem",
			ServerCertificate: "/tmp/access/server.pem",
			ServerKey:         "/tmp/access/server-key.pem",
		},
	}
	got := HTTPGatewayComposeService(files, HTTPGatewaySpec{ServiceName: "openbao-access", ContainerPort: 8443})
	for _, want := range []string{
		"cap_drop: [\"ALL\"]",
		"entrypoint: [\"/bin/sh\", \"-ec\"]",
		"exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway compose missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"cap_add:", "/run/baseharbor", "cat /usr/bin/caddy"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("gateway compose contains unnecessary runtime privilege/staging %q:\n%s", forbidden, got)
		}
	}

	privilegedPort := HTTPGatewayComposeService(files, HTTPGatewaySpec{ServiceName: "https-access", ContainerPort: 443})
	if !strings.Contains(privilegedPort, "cap_add: [\"NET_BIND_SERVICE\"]") {
		t.Fatalf("gateway on privileged port must add NET_BIND_SERVICE:\n%s", privilegedPort)
	}
}
