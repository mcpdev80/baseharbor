package devgateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func VerifyHosts(ctx context.Context, target string, hosts []string) error {
	wanted := map[string]struct{}{}
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			wanted[host] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(files.CA)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return errors.New("development gateway CA contains no certificates")
	}
	seen := map[string]struct{}{}
	var errs []error
	for _, route := range current.Routes {
		if _, ok := wanted[route.Host]; !ok {
			continue
		}
		seen[route.Host] = struct{}{}
		allowed := relatedRedirectURLs(current.Routes, route, current.HostPort)
		if err := verifyRoute(ctx, roots, route, current.HostPort, allowed...); err != nil {
			errs = append(errs, err)
		}
		if current.IPAccessAddress != "" && route.DirectIPPort > 0 {
			if err := verifyDirectIPRoute(ctx, roots, current, route); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for host := range wanted {
		if _, ok := seen[host]; !ok {
			errs = append(errs, fmt.Errorf("%s is not registered in the development gateway", host))
		}
	}
	return errors.Join(errs...)
}

func verifyDirectIPRoute(ctx context.Context, roots *x509.CertPool, current state, route Route) error {
	address := current.IPAccessAddress
	port := route.DirectIPPort
	directURL := "https://" + net.JoinHostPort(address, strconv.Itoa(port)) + "/"
	allowed := make([]string, 0, len(current.Routes))
	for _, candidate := range current.Routes {
		if candidate.DirectIPPort > 0 {
			allowed = append(allowed, "https://"+net.JoinHostPort(address, strconv.Itoa(candidate.DirectIPPort)))
		}
	}
	dialer := &net.Dialer{}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: address},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(address, strconv.Itoa(port)))
		},
	}
	client := &http.Client{Transport: transport}
	if err := serviceaccess.VerifyBrowserRouteWithAllowedAuthorities(ctx, client, directURL, allowed...); err != nil {
		return routeVerificationFailure("management UI IP "+route.ManagementKey, err)
	}
	return nil
}

func relatedRedirectURLs(routes []Route, route Route, hostPort int) []string {
	if !strings.HasSuffix(route.Key, "/admin") && !strings.HasSuffix(route.Key, "/identity-admin") {
		return nil
	}
	for _, candidate := range routes {
		if candidate.Owner != route.Owner {
			continue
		}
		if strings.HasSuffix(candidate.Key, "/login") || strings.HasSuffix(candidate.Key, "/identity") {
			return []string{canonicalURL(candidate.Host, hostPort)}
		}
	}
	return nil
}

func verifyRoute(ctx context.Context, roots *x509.CertPool, route Route, hostPort int, allowedURLs ...string) error {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			ServerName: route.Host,
		},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)))
		},
	}
	client := &http.Client{Transport: transport}
	if err := serviceaccess.VerifyBrowserRouteWithAllowedAuthorities(ctx, client, canonicalURL(route.Host, hostPort)+"/", allowedURLs...); err != nil {
		return routeVerificationFailure(route.Host, err)
	}
	return nil
}

func routeVerificationFailure(host string, err error) error {
	if err == nil {
		return nil
	}
	for _, status := range []string{"502", "503", "504"} {
		if strings.Contains(err.Error(), "browser surface final response is HTTP "+status) {
			return fmt.Errorf("%s: application upstream unavailable (HTTP %s); verify the workload listener matches exposure.http.port and the selected Target network is reachable", host, status)
		}
	}
	return fmt.Errorf("%s: %w", host, err)
}

func Verify(ctx context.Context, target string) error {
	routes, err := Routes(target)
	if err != nil {
		return err
	}
	hosts := make([]string, 0, len(routes))
	for _, route := range routes {
		hosts = append(hosts, route.Host)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var last error
	for {
		if err := VerifyHosts(verifyCtx, target, hosts); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-verifyCtx.Done():
			if last != nil {
				return last
			}
			return verifyCtx.Err()
		case <-ticker.C:
		}
	}
}
