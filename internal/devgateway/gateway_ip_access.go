package devgateway

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const ipAccessAddressEnv = "BASEHARBOR_DEV_GATEWAY_IP_ACCESS_ADDRESS"

func gatewayIPAccessAddress(routes []Route, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = strings.TrimSpace(os.Getenv(ipAccessAddressEnv))
	}
	if configured == "" {
		return "", nil
	}
	ip := net.ParseIP(configured)
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.To4() == nil {
		return "", fmt.Errorf("%s must be an explicit non-loopback IPv4 address", ipAccessAddressEnv)
	}
	for _, route := range routes {
		if route.ManagementKey != "" {
			return ip.String(), nil
		}
	}
	return "", nil
}

func assignDirectIPPorts(routes []Route, address, previousAddress string, materialized bool, reserved ...int) ([]Route, error) {
	used := map[int]struct{}{}
	for _, port := range reserved {
		used[port] = struct{}{}
	}
	for i := range routes {
		route := &routes[i]
		if address == "" || route.ManagementKey == "" {
			route.DirectIPPort = 0
			continue
		}
		if previousAddress == address && route.DirectIPPort > 0 {
			used[route.DirectIPPort] = struct{}{}
			if materialized || directIPPortAvailable(address, route.DirectIPPort) {
				continue
			}
		}
		port, err := allocateDirectIPPort(address, used)
		if err != nil {
			return nil, fmt.Errorf("allocate direct IP listener for management UI %s: %w", route.ManagementKey, err)
		}
		route.DirectIPPort = port
		used[port] = struct{}{}
	}
	return routes, nil
}

func allocateDirectIPPort(address string, excluded map[int]struct{}) (int, error) {
	for attempt := 0; attempt < 32; attempt++ {
		// The runtime may be remote, so the target IP need not be assigned to
		// the workstation executing baha. Reserve a distinct dynamic candidate
		// locally; the runtime's port-conflict response remains authoritative.
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", "0"))
		if err != nil {
			return 0, err
		}
		_, value, splitErr := net.SplitHostPort(listener.Addr().String())
		port, parseErr := strconv.Atoi(value)
		if splitErr != nil {
			parseErr = splitErr
		}
		_ = listener.Close()
		if parseErr != nil {
			return 0, parseErr
		}
		if _, duplicate := excluded[port]; !duplicate {
			return port, nil
		}
	}
	return 0, errors.New("could not allocate a distinct ephemeral TCP port")
}

func directIPPortAvailable(address string, port int) bool {
	if net.ParseIP(address) == nil || port < 1 || port > 65535 {
		return false
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
