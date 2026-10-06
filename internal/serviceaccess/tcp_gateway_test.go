package serviceaccess

import (
	"strings"
	"testing"
)

func TestTCPGatewayConfigTerminatesTLS(t *testing.T) {
	got := tcpGatewayConfig(TCPGatewaySpec{UpstreamHost: "postgres", UpstreamPort: 5432, ContainerPort: 5432})
	for _, want := range []string{"mode tcp", "bind :5432 ssl crt", "server provider postgres:5432 check", "ssl-min-ver TLSv1.2", "bind 127.0.0.1:8404", "stats uri /stats", "option log-health-checks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("TCP gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestTCPGatewayConfigSupportsMultipleHealthyMembers(t *testing.T) {
	got := tcpGatewayConfig(TCPGatewaySpec{
		ContainerPort: 5672,
		Upstreams: []TCPGatewayUpstream{
			{Name: "rabbit-1", Host: "rabbitmq-default-1", Port: 5672},
			{Name: "rabbit-2", Host: "rabbitmq-default-2", Port: 5672},
			{Name: "rabbit-3", Host: "rabbitmq-default-3", Port: 5672},
		},
	})
	for _, want := range []string{
		"balance roundrobin",
		"default-server resolvers runtime-dns resolve-prefer ipv4 init-addr last,libc,none on-marked-down shutdown-sessions",
		"resolvers runtime-dns\n  parse-resolv-conf",
		"server rabbit-1 rabbitmq-default-1:5672 check",
		"server rabbit-2 rabbitmq-default-2:5672 check",
		"server rabbit-3 rabbitmq-default-3:5672 check",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("TCP gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestTCPGatewaySupportsProviderAwareHealthChecksWithoutEmbeddingSecret(t *testing.T) {
	spec := TCPGatewaySpec{
		ServiceName:      "valkey-access",
		ContainerPort:    6379,
		PublishedPortEnv: "VALKEY_HOST_PORT",
		Environment: map[string]string{
			"VALKEY_HEALTH_PASSWORD": "${VALKEY_PASSWORD}",
		},
		Upstreams: []TCPGatewayUpstream{
			{Name: "valkey", Host: "valkey", Port: 6379},
			{Name: "valkey-2", Host: "valkey-2", Port: 6379},
		},
		BackendDirectives: []string{
			"option tcp-check",
			"tcp-check connect",
			"tcp-check send-lf \"AUTH %[env(VALKEY_HEALTH_PASSWORD)]\\r\"",
			"tcp-check expect string +OK",
			"tcp-check send-lf \"INFO replication\\r\"",
			"tcp-check expect string role:master",
			"server valkey valkey:6379 check init-state fully-down inter 1s rise 1 fall 1",
			"server valkey-2 valkey-2:6379 check init-state fully-down inter 1s rise 1 fall 1",
		},
		ServerDirectives: []string{"init-state fully-down", "inter 1s", "rise 1", "fall 1"},
	}
	cfg := tcpGatewayConfig(spec)
	for _, want := range []string{
		"option tcp-check",
		"tcp-check connect",
		"tcp-check send-lf \"AUTH %[env(VALKEY_HEALTH_PASSWORD)]\\r\"",
		"tcp-check expect string role:master",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("TCP gateway config missing %q:\n%s", want, cfg)
		}
	}
	if strings.Contains(cfg, "super-secret") {
		t.Fatal("TCP gateway config embeds provider secret")
	}
	compose := TCPGatewayComposeService(TCPGatewayFiles{
		Config: "/tmp/haproxy.cfg",
		PEM:    "/tmp/server.pem",
	}, spec)
	if !strings.Contains(compose, "VALKEY_HEALTH_PASSWORD: \"${VALKEY_PASSWORD}\"") {
		t.Fatalf("TCP gateway compose missing secret environment projection:\n%s", compose)
	}
}
