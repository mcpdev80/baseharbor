package devgateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// BrowserClient verifies canonical TLS names while dialing the owning gateway's
// recorded listener. Core-local readiness must not require workstation DNS/hosts.
func BrowserClient(target, host string) (*http.Client, error) {
	files, err := FilesFor(target)
	if err != nil {
		return nil, err
	}
	current, err := loadState(files.State)
	if err != nil {
		return nil, err
	}
	owner := ""
	host = strings.ToLower(strings.TrimSpace(host))
	for _, route := range current.Routes {
		if route.Host == host {
			owner = route.Owner
			break
		}
	}
	if owner == "" {
		return nil, errors.New("browser host is not registered to an owned gateway route")
	}
	allowed := map[string]bool{}
	for _, route := range current.Routes {
		if route.Owner == owner {
			allowed[route.Host] = true
		}
	}
	pem, err := os.ReadFile(files.CA)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("gateway CA contains no certificates")
	}
	address := current.BindAddress
	if address == "" {
		address = "127.0.0.1"
	}
	port := strconv.Itoa(current.HostPort)
	dialer := &net.Dialer{}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DialContext: func(ctx context.Context, network, destination string) (net.Conn, error) {
		name, requestedPort, err := net.SplitHostPort(destination)
		if err != nil {
			return nil, err
		}
		if !allowed[name] || requestedPort != port {
			return nil, errors.New("browser redirect is outside the owned gateway authority")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(address, port))
	}}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}
