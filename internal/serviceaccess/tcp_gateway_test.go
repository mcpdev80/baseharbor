package serviceaccess

import (
	"strings"
	"testing"
)

func TestTCPGatewayConfigTerminatesTLS(t *testing.T) {
	got := tcpGatewayConfig(TCPGatewaySpec{UpstreamHost: "postgres", UpstreamPort: 5432, ContainerPort: 5432})
	for _, want := range []string{"mode tcp", "bind :5432 ssl crt", "server provider postgres:5432 check", "ssl-min-ver TLSv1.2"} {
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
		"server rabbit-1 rabbitmq-default-1:5672 check",
		"server rabbit-2 rabbitmq-default-2:5672 check",
		"server rabbit-3 rabbitmq-default-3:5672 check",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("TCP gateway config missing %q:\n%s", want, got)
		}
	}
}
