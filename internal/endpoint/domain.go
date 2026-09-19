package endpoint

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Endpoint is a stable logical service endpoint. Logical identity is independent
// from runtime-generated container or pod names.
type Endpoint struct {
	Service string `json:"service"`
	Scheme  string `json:"scheme"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// ExposureStatus is machine-readable readiness for one observed or managed HTTP
// exposure.
type ExposureStatus struct {
	Endpoint
	Ready  bool   `json:"ready"`
	Detail string `json:"detail,omitempty"`
}

func (e Endpoint) Validate() error {
	if strings.TrimSpace(e.Service) == "" {
		return fmt.Errorf("endpoint service is required")
	}
	switch e.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("endpoint scheme %q is unsupported", e.Scheme)
	}
	if strings.TrimSpace(e.Host) == "" {
		return fmt.Errorf("endpoint host is required")
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("endpoint port %d is invalid", e.Port)
	}
	return nil
}

func HTTPPortScheme(targetPort, publishedPort int) (string, bool) {
	for _, port := range []int{targetPort, publishedPort} {
		switch port {
		case 443, 8443:
			return "https", true
		}
	}
	for _, port := range []int{targetPort, publishedPort} {
		switch port {
		case 80, 3000, 3001, 5000, 8000, 8080, 8081, 8888:
			return "http", true
		}
	}
	return "", false
}

func NormalizePublishedHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return "127.0.0.1"
	}
	return strings.Trim(host, "[]")
}

func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func ProbeHTTP(ctx context.Context, ep Endpoint) ExposureStatus {
	status := ExposureStatus{Endpoint: ep}
	if err := ep.Validate(); err != nil {
		status.Detail = "invalid endpoint"
		return status
	}
	dialAddress := net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))
	requestHost := ep.Host
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, dialAddress)
		},
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ServerName:         requestHost,
			InsecureSkipVerify: true, // readiness verifies reachability; trust policy is a separate binding concern.
		},
		TLSHandshakeTimeout: 2 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	url := ep.Scheme + "://" + net.JoinHostPort(requestHost, strconv.Itoa(ep.Port)) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		status.Detail = "invalid endpoint"
		return status
	}
	resp, err := client.Do(req)
	if err != nil {
		status.Detail = "unreachable"
		return status
	}
	defer resp.Body.Close()
	status.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	status.Ready = resp.StatusCode < 500
	return status
}

// ProbeHTTPDialTarget verifies a logical HTTP endpoint while dialing a separate
// local/runtime address. This supports hostname-bound TLS without conflating
// endpoint identity with runtime placement.
func ProbeHTTPDialTarget(ctx context.Context, logical Endpoint, dialHost string, dialPort int) ExposureStatus {
	status := ExposureStatus{Endpoint: logical}
	if err := logical.Validate(); err != nil || strings.TrimSpace(dialHost) == "" || dialPort < 1 || dialPort > 65535 {
		status.Detail = "invalid endpoint"
		return status
	}
	dialAddress := net.JoinHostPort(dialHost, strconv.Itoa(dialPort))
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, dialAddress)
		},
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ServerName:         logical.Host,
			InsecureSkipVerify: true,
		},
		TLSHandshakeTimeout: 2 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	url := logical.Scheme + "://" + net.JoinHostPort(logical.Host, strconv.Itoa(logical.Port)) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		status.Detail = "invalid endpoint"
		return status
	}
	resp, err := client.Do(req)
	if err != nil {
		status.Detail = "unreachable"
		return status
	}
	defer resp.Body.Close()
	status.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	status.Ready = resp.StatusCode < 500
	return status
}

func SortExposureStatuses(values []ExposureStatus) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Service != values[j].Service {
			return values[i].Service < values[j].Service
		}
		if values[i].Port != values[j].Port {
			return values[i].Port < values[j].Port
		}
		return values[i].Scheme < values[j].Scheme
	})
}
