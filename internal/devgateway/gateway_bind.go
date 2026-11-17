package devgateway

import (
	"errors"
	"net"
	"os"
	"strings"
)

// gatewayBindAddress deliberately requires a real routable IP and explicit
// target-scoped DNS names before a gateway can leave the loopback interface.
// TLS remains mandatory and is issued by the existing managed service PKI;
// the workstation must trust its actual CA and resolve the canonical hosts.
func gatewayBindAddress(routes []Route) (string, error) {
	return validatedGatewayBindAddress(routes, strings.TrimSpace(os.Getenv("BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS")))
}

func validatedGatewayBindAddress(routes []Route, host string) (string, error) {
	if host == "" || host == "127.0.0.1" {
		return "127.0.0.1", nil
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.To4() == nil {
		return "", errors.New("development gateway LAN listener requires an explicit non-loopback IPv4 address; wildcard and hostname binds are forbidden")
	}
	for _, route := range routes {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(route.Host), "."))
		if name == "localhost" || strings.HasSuffix(name, ".localhost") {
			return "", errors.New("development gateway LAN listener requires a configured resolvable target domain; *.localhost is loopback-only (configure: baha dev domain DOMAIN)")
		}
	}
	return ip.String(), nil
}

// A selected Core's concrete IP plus explicitly configured non-localhost names
// permits canonical URLs from its workstation. Explicit bind overrides win.
func gatewayBindAddressForIPAccess(routes []Route, coreIP string) (string, error) {
	if strings.TrimSpace(os.Getenv("BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS")) != "" || coreIP == "" || len(routes) == 0 {
		return gatewayBindAddress(routes)
	}
	for _, route := range routes {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(route.Host), "."))
		if name == "localhost" || strings.HasSuffix(name, ".localhost") {
			return "127.0.0.1", nil
		}
	}
	return validatedGatewayBindAddress(routes, coreIP)
}
