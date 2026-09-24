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
