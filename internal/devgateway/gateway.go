package devgateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	stateVersion             = 1
	gatewayPort              = 443
	rootlessGatewayPort      = 8443
	fallbackGatewayPortStart = 18443
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type runtimeLocalHTTPS interface {
	PreferredLocalHTTPSPort() int
}

type Route struct {
	Owner         string `json:"owner"`
	Key           string `json:"key"`
	Host          string `json:"host"`
	PathPrefix    string `json:"path_prefix,omitempty"`
	Upstream      string `json:"upstream"`
	Network       string `json:"network"`
	TrustFile     string `json:"trust_file"`
	ServerName    string `json:"server_name"`
	ManagementKey string `json:"management_key,omitempty"`
	DirectIPPort  int    `json:"direct_ip_port,omitempty"`
}

type OwnerRoutes struct {
	Owner  string
	Routes []Route
}

type state struct {
	Version         int     `json:"version"`
	HostPort        int     `json:"host_port,omitempty"`
	IPAccessAddress string  `json:"ip_access_address,omitempty"`
	Routes          []Route `json:"routes"`
}

type Files struct {
	Dir       string
	State     string
	Compose   string
	Env       string
	Caddyfile string
	Project   string
	Cert      string
	Key       string
	CA        string
}

func FilesFor(target string) (Files, error) {
	root, err := deployment.TargetStateRoot(strings.TrimSpace(target))
	if err != nil {
		return Files{}, err
	}
	dir := filepath.Join(root, "developer-access", "dev", "gateway")
	return Files{
		Dir:       dir,
		State:     filepath.Join(dir, "routes.json"),
		Compose:   filepath.Join(dir, "compose.yaml"),
		Env:       filepath.Join(dir, "runtime.env"),
		Caddyfile: filepath.Join(dir, "runtime", "Caddyfile"),
		Project:   bhruntime.ApplicationProjectName(target, "dev-gateway", "dev"),
		Cert:      filepath.Join(dir, "runtime", "server.pem"),
		Key:       filepath.Join(dir, "runtime", "server-key.pem"),
		CA:        filepath.Join(dir, "runtime", "ca.pem"),
	}, nil
}

func ReplaceOwnerRoutes(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target, owner string, routes []Route) error {
	return ReplaceRoutes(ctx, runtime, issuer, target, OwnerRoutes{Owner: owner, Routes: routes})
}

func ReplaceRoutes(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string, groups ...OwnerRoutes) error {
	return replaceRoutes(ctx, runtime, issuer, target, nil, groups...)
}

func ReplaceRoutesWithIPAccessAddress(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target, ipAddress string, groups ...OwnerRoutes) error {
	return replaceRoutes(ctx, runtime, issuer, target, &ipAddress, groups...)
}

func replaceRoutes(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string, ipAddress *string, groups ...OwnerRoutes) error {
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if errors.Is(err, os.ErrNotExist) {
		current = state{Version: stateVersion}
	} else if err != nil {
		return err
	}
	previous := cloneState(current)
	owners := map[string]struct{}{}
	for _, group := range groups {
		owner := strings.TrimSpace(group.Owner)
		if owner == "" {
			return errors.New("development gateway route owner is required")
		}
		owners[owner] = struct{}{}
	}
	filtered := current.Routes[:0]
	for _, route := range current.Routes {
		if _, replace := owners[route.Owner]; !replace {
			filtered = append(filtered, route)
		}
	}
	for _, group := range groups {
		owner := strings.TrimSpace(group.Owner)
		for _, route := range group.Routes {
			route.Owner = owner
			if err := validateRoute(route); err != nil {
				return err
			}
			filtered = append(filtered, route)
		}
	}
	current.Routes = normalizedRoutes(filtered)
	if len(current.Routes) > 0 && current.HostPort == 0 {
		current.HostPort, err = resolveGatewayHostPort(files, current, runtime)
		if err != nil {
			return err
		}
	}
	if err := saveRouteStateForReconcile(ctx, runtime, files, previous, current); err != nil {
		return err
	}
	return reconcile(ctx, runtime, issuer, target, ipAddress)
}

func UpsertOwnerRoutes(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target, owner string, routes []Route) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return errors.New("development gateway route owner is required")
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if errors.Is(err, os.ErrNotExist) {
		current = state{Version: stateVersion}
	} else if err != nil {
		return err
	}
	previous := cloneState(current)
	byKey := map[string]Route{}
	for _, route := range current.Routes {
		byKey[route.Key] = route
	}
	for _, route := range routes {
		route.Owner = owner
		if err := validateRoute(route); err != nil {
			return err
		}
		byKey[route.Key] = route
	}
	current.Routes = current.Routes[:0]
	for _, route := range byKey {
		current.Routes = append(current.Routes, route)
	}
	current.Routes = normalizedRoutes(current.Routes)
	if len(current.Routes) > 0 && current.HostPort == 0 {
		current.HostPort, err = resolveGatewayHostPort(files, current, runtime)
		if err != nil {
			return err
		}
	}
	if err := saveRouteStateForReconcile(ctx, runtime, files, previous, current); err != nil {
		return err
	}
	return Reconcile(ctx, runtime, issuer, target)
}

func cloneState(input state) state {
	cloned := input
	cloned.Routes = append([]Route(nil), input.Routes...)
	return cloned
}

func saveRouteStateForReconcile(ctx context.Context, runtime Runtime, files Files, previous, next state) error {
	// Route/network changes are reconciled in place by Reconcile through
	// ConfigProject + UpProject. Do not destroy the currently healthy gateway
	// before the replacement configuration has even been validated. Compose
	// may still need to recreate the single ingress container when its attached
	// network set changes, but the existing capacity remains available until
	// the validated reconciliation actually starts. This is the strongest
	// continuity possible on a single-host runtime where one loopback host port
	// has a single owner.
	if len(previous.Routes) > 0 && len(next.Routes) == 0 {
		if err := retireLastGatewayRoutes(ctx, runtime, files, previous); err != nil {
			return fmt.Errorf("retire last development gateway routes; preserve route ownership for retry: %w", err)
		}
	}
	return saveState(files.State, next)
}

func routeNetworkSetChanged(before, after []Route) bool {
	left := routeNetworkSet(before)
	right := routeNetworkSet(after)
	if len(left) != len(right) {
		return true
	}
	for network := range left {
		if _, ok := right[network]; !ok {
			return true
		}
	}
	return false
}

func routeNetworkSet(routes []Route) map[string]struct{} {
	result := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if network := strings.TrimSpace(route.Network); network != "" {
			result[network] = struct{}{}
		}
	}
	return result
}

// OwnerRoutesPresent is a read-only teardown preflight. Missing registration is
// safe only when no gateway projection or owned runtime resources remain.
// Other owners' valid registrations never authorize reconciliation for this app.
func OwnerRoutesPresent(ctx context.Context, runtime Runtime, target, owner string) (bool, error) {
	files, err := FilesFor(target)
	if err != nil {
		return false, err
	}
	current, err := loadState(files.State)
	if err == nil {
		for _, route := range current.Routes {
			if route.Owner == owner {
				return true, nil
			}
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entries, err := os.ReadDir(files.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if len(entries) > 0 {
		return false, errors.New("development gateway registration is missing but retained state exists; run baha doctor before retrying destroy")
	}
	inventory, ok := runtime.(interface {
		ListOwnedProjectResources(context.Context, string) ([]bhruntime.ProjectResource, error)
	})
	if !ok {
		return false, errors.New("development gateway absence cannot be verified by this runtime")
	}
	resources, err := inventory.ListOwnedProjectResources(ctx, files.Project)
	if err != nil {
		return false, fmt.Errorf("inspect development gateway ownership: %w", err)
	}
	if len(resources) > 0 {
		return false, errors.New("development gateway resources remain without their registration; repair the gateway state before retrying destroy")
	}
	return false, nil
}

func RemoveOwners(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string, owners ...string) error {
	groups := make([]OwnerRoutes, 0, len(owners))
	for _, owner := range owners {
		if strings.TrimSpace(owner) != "" {
			groups = append(groups, OwnerRoutes{Owner: owner})
		}
	}
	if len(groups) == 0 {
		return nil
	}
	return ReplaceRoutes(ctx, runtime, issuer, target, groups...)
}

func DestroyTarget(ctx context.Context, runtime Runtime, target string) error {
	if runtime == nil {
		return errors.New("development gateway runtime is required")
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	if _, err := os.Stat(files.Compose); err == nil {
		if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("destroy development gateway: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func Reconcile(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string) error {
	return reconcile(ctx, runtime, issuer, target, nil)
}

func reconcile(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string, requestedIPAccessAddress *string) error {
	if runtime == nil {
		return errors.New("development gateway runtime is required")
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pruned, changed, err := pruneUnavailableTrustRoutes(current.Routes)
	if err != nil {
		return err
	}
	if changed {
		previous := cloneState(current)
		current.Routes = pruned
		if err := saveRouteStateForReconcile(ctx, runtime, files, previous, current); err != nil {
			return err
		}
	}
	if len(current.Routes) == 0 {
		return verifyEmptyGatewayInactive(ctx, runtime, files)
	}
	ipAddress := current.IPAccessAddress
	if requestedIPAccessAddress != nil {
		ipAddress = *requestedIPAccessAddress
	} else if configured := strings.TrimSpace(os.Getenv(ipAccessAddressEnv)); configured != "" {
		ipAddress = configured
	}
	ipAddress, err = gatewayIPAccessAddress(current.Routes, ipAddress)
	if err != nil {
		return err
	}
	if issuer == nil {
		return errors.New("development gateway issuer is required")
	}
	if err := os.MkdirAll(filepath.Join(files.Dir, "runtime", "trust"), 0o755); err != nil {
		return err
	}
	materialized := false
	if _, statErr := os.Stat(files.Compose); statErr == nil {
		materialized = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect development gateway runtime: %w", statErr)
	}
	hostPort, err := resolveGatewayHostPort(files, current, runtime)
	if err != nil {
		return err
	}
	current.Routes, err = assignDirectIPPorts(current.Routes, ipAddress, current.IPAccessAddress, materialized, hostPort)
	if err != nil {
		return err
	}
	current.HostPort = hostPort
	current.IPAccessAddress = ipAddress
	hosts := make([]string, 0, len(current.Routes))
	for _, route := range current.Routes {
		hosts = append(hosts, route.Host)
	}
	if ipAddress != "" {
		hosts = append(hosts, ipAddress)
	}
	policy, err := serviceaccess.Resolve("dev", "dev-gateway", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	policy.ServerName = hosts[0]
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(files.Dir, "pki"), hosts...)
	if err != nil {
		return fmt.Errorf("prepare development gateway TLS: %w", err)
	}
	if err := projectReadable(material.CA, files.CA); err != nil {
		return err
	}
	if err := projectReadable(material.ServerCertificate, files.Cert); err != nil {
		return err
	}
	// This is a derived runtime projection mounted read-only into an
	// unprivileged container. Rootless Podman remaps container UIDs, so a 0600
	// host-owned projection is unreadable even though the authoritative PKI key
	// remains protected under pki/. Match the service-access gateway projection
	// model and make only this read-only runtime copy container-readable.
	if err := projectReadableMode(material.ServerKey, files.Key, 0o644); err != nil {
		return err
	}
	trustTargets := map[string]string{}
	for i, route := range current.Routes {
		if !strings.HasPrefix(route.Upstream, "https://") {
			continue
		}
		targetPath := filepath.Join(files.Dir, "runtime", "trust", fmt.Sprintf("route-%03d.pem", i))
		if err := projectReadable(route.TrustFile, targetPath); err != nil {
			return fmt.Errorf("project trust for %s: %w", route.Key, err)
		}
		trustTargets[route.Key] = targetPath
	}
	// Remote workstation access is opt-in. Never expose the default
	// *.localhost development names on a LAN listener: remote hosts resolve
	// those names to themselves. The selected bind address must be one
	// concrete non-loopback IP on the Core host, not a wildcard.
	bindHost, err := gatewayBindAddress(current.Routes)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 5; attempt++ {
		if err := writeGatewayRuntimeConfiguration(ctx, runtime, files, current, trustTargets, hostPort, bindHost, ipAddress); err != nil {
			return err
		}
		if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err == nil {
			return nil
		} else if ipAddress == "" || !bhruntime.IsPortBindingConflict(err) || attempt == 4 {
			return fmt.Errorf("start development gateway on HTTPS port %d: %w", hostPort, err)
		}
		for i := range current.Routes {
			current.Routes[i].DirectIPPort = 0
		}
		current.Routes, err = assignDirectIPPorts(current.Routes, ipAddress, "", false, hostPort)
		if err != nil {
			return err
		}
	}
	return errors.New("development gateway direct-IP listeners exhausted port allocation retries")
}

func writeGatewayRuntimeConfiguration(ctx context.Context, runtime Runtime, files Files, current state, trust map[string]string, hostPort int, bindHost, ipAddress string) error {
	if err := os.WriteFile(files.Caddyfile, []byte(renderCaddyfile(current.Routes, hostPort, ipAddress)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(files.Env, nil, 0o600); err != nil {
		return err
	}
	if err := saveState(files.State, current); err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(renderComposeBound(files, current.Routes, trust, hostPort, bindHost, ipAddress)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate development gateway: %w", err)
	}
	return nil
}

func URL(host string) string {
	return canonicalURL(host, gatewayPort)
}

func URLForTarget(target, host string) string {
	files, err := FilesFor(target)
	if err != nil {
		return URL(host)
	}
	current, err := loadState(files.State)
	if err != nil {
		return URL(host)
	}
	return canonicalURL(host, current.HostPort)
}

func ManagementURLs(target, owner string, desired []Route) (map[string]string, map[string]string, error) {
	files, err := FilesFor(target)
	if err != nil {
		return nil, nil, err
	}
	current, err := loadState(files.State)
	if err != nil {
		return nil, nil, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, nil, errors.New("development management URL owner is required")
	}
	wanted := make(map[string]string, len(desired))
	for _, route := range desired {
		if route.ManagementKey == "" {
			continue
		}
		if previous, duplicate := wanted[route.Key]; duplicate && previous != route.ManagementKey {
			return nil, nil, fmt.Errorf("conflicting development management keys for route %q", route.Key)
		}
		wanted[route.Key] = route.ManagementKey
	}
	selected := map[string]Route{}
	priority := map[string]int{}
	for _, route := range current.Routes {
		if route.ManagementKey == "" {
			continue
		}
		managementKey, required := wanted[route.Key]
		if !required || managementKey != route.ManagementKey {
			continue
		}
		candidatePriority := 0
		switch {
		case route.Owner == owner:
			candidatePriority = 2
		case strings.HasPrefix(route.Owner, "shared/"):
			candidatePriority = 1
		default:
			continue
		}
		if candidatePriority < priority[route.ManagementKey] {
			continue
		}
		if candidatePriority == priority[route.ManagementKey] {
			return nil, nil, fmt.Errorf("duplicate development management key %q for owner %q", route.ManagementKey, route.Owner)
		}
		selected[route.ManagementKey] = route
		priority[route.ManagementKey] = candidatePriority
	}
	canonical := make(map[string]string, len(selected))
	directIP := make(map[string]string, len(selected))
	for key, route := range selected {
		canonical[key] = canonicalURL(route.Host, current.HostPort)
		if current.IPAccessAddress != "" && route.DirectIPPort > 0 {
			directIP[key] = "https://" + net.JoinHostPort(current.IPAccessAddress, strconv.Itoa(route.DirectIPPort))
		}
	}
	return canonical, directIP, nil
}

// URLForRuntime returns the effective canonical gateway URL before or after
// gateway state exists. Persisted state remains authoritative; on first
// reconciliation the runtime determines the same deterministic host port that
// Reconcile will persist (Docker 443, rootless Podman 8443).
func URLForRuntime(target, host string, runtime Runtime) string {
	return urlForRuntime(target, host, runtime, gatewayPortAvailable)
}

func urlForRuntime(target, host string, runtime Runtime, available func(int) bool) string {
	files, err := FilesFor(target)
	if err == nil {
		if current, loadErr := loadState(files.State); loadErr == nil && current.HostPort > 0 {
			return canonicalURL(host, current.HostPort)
		}
	}
	preferred := gatewayHostPort(runtime)
	selected, selectErr := selectGatewayHostPort(preferred, 0, false, available)
	if selectErr != nil {
		selected = preferred
	}
	return canonicalURL(host, selected)
}

func canonicalURL(host string, port int) string {
	host = strings.TrimSpace(host)
	if port == 0 || port == gatewayPort {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(port)
}

func gatewayHostPort(runtime Runtime) int {
	if capable, ok := runtime.(runtimeLocalHTTPS); ok {
		if port := capable.PreferredLocalHTTPSPort(); port > 0 {
			return port
		}
	}
	return gatewayPort
}

func resolveGatewayHostPort(files Files, current state, runtime Runtime) (int, error) {
	materialized := false
	if _, err := os.Stat(files.Compose); err == nil {
		materialized = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect development gateway runtime: %w", err)
	}
	return selectGatewayHostPort(gatewayHostPort(runtime), current.HostPort, materialized, gatewayPortAvailable)
}

func selectGatewayHostPort(preferred, persisted int, materialized bool, available func(int) bool) (int, error) {
	if persisted > 0 {
		if materialized || available(persisted) {
			return persisted, nil
		}
	}
	if preferred > 0 && available(preferred) {
		return preferred, nil
	}
	for port := fallbackGatewayPortStart; port <= 65535; port++ {
		if port == preferred || port == persisted {
			continue
		}
		if available(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("development gateway preferred HTTPS port %d is unavailable and no free fallback port was found", preferred)
}

func gatewayPortAvailable(port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func Routes(target string) ([]Route, error) {
	files, err := FilesFor(target)
	if err != nil {
		return nil, err
	}
	current, err := loadState(files.State)
	if err != nil {
		return nil, err
	}
	return append([]Route(nil), current.Routes...), nil
}

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

func loadState(path string) (state, error) {
	expected, err := os.Lstat(path)
	if err != nil {
		return state{}, err
	}
	if !expected.Mode().IsRegular() || expected.Mode().Perm()&0077 != 0 {
		return state{}, errors.New("development gateway route owner must be a protected regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return state{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 || opened.Size() > 4<<20 {
		return state{}, errors.New("development gateway route owner is replaced, unprotected or oversized")
	}
	var value state
	decoder := json.NewDecoder(io.LimitReader(file, (4<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return state{}, fmt.Errorf("decode development gateway routes: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return state{}, errors.New("development gateway route owner contains trailing data")
	}
	if value.Version != stateVersion {
		return state{}, fmt.Errorf("unsupported development gateway route state version %d", value.Version)
	}
	value.Routes = normalizedRoutes(value.Routes)
	return value, nil
}

func saveState(path string, value state) error {
	value.Version = stateVersion
	value.Routes = normalizedRoutes(value.Routes)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	directory, err := os.Lstat(filepath.Dir(path))
	if err != nil || !directory.IsDir() || directory.Mode()&os.ModeSymlink != 0 {
		return errors.New("development gateway route owner directory is noncanonical")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".routes-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return err
	}
	return nil
}

func pruneUnavailableTrustRoutes(routes []Route) ([]Route, bool, error) {
	out := make([]Route, 0, len(routes))
	changed := false
	for _, route := range routes {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(route.Upstream)), "https://") {
			if _, err := os.Stat(strings.TrimSpace(route.TrustFile)); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					changed = true
					continue
				}
				return nil, false, fmt.Errorf("inspect development gateway trust for %s: %w", route.Key, err)
			}
		}
		out = append(out, route)
	}
	return out, changed, nil
}

func validateRoute(route Route) error {
	for label, value := range map[string]string{
		"key": route.Key, "host": route.Host, "upstream": route.Upstream,
		"network": route.Network,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("development gateway route %s is required", label)
		}
	}
	if !strings.HasPrefix(route.Upstream, "https://") && !strings.HasPrefix(route.Upstream, "http://") {
		return fmt.Errorf("development gateway route %s must use an HTTP(S) upstream", route.Key)
	}
	if strings.HasPrefix(route.Upstream, "https://") {
		if strings.TrimSpace(route.TrustFile) == "" || strings.TrimSpace(route.ServerName) == "" {
			return fmt.Errorf("development gateway HTTPS route %s requires trust file and server name", route.Key)
		}
	}
	if strings.ContainsAny(route.Host, "/:@ \t\r\n") {
		return fmt.Errorf("development gateway host %q is invalid", route.Host)
	}
	if route.ManagementKey != "" {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(route.ManagementKey) {
			return fmt.Errorf("development gateway management key %q is invalid", route.ManagementKey)
		}
		if route.PathPrefix != "" {
			return fmt.Errorf("development gateway management route %s cannot use a path prefix", route.Key)
		}
	}
	if route.DirectIPPort < 0 || route.DirectIPPort > 65535 {
		return fmt.Errorf("development gateway direct-IP port for %s is invalid", route.Key)
	}
	return nil
}

func normalizedRoutes(routes []Route) []Route {
	byKey := map[string]Route{}
	for _, route := range routes {
		route.Owner = strings.TrimSpace(route.Owner)
		route.Key = strings.TrimSpace(route.Key)
		route.Host = strings.ToLower(strings.TrimSpace(route.Host))
		route.PathPrefix = strings.TrimSpace(route.PathPrefix)
		if route.PathPrefix != "" {
			route.PathPrefix = "/" + strings.Trim(strings.TrimSpace(route.PathPrefix), "/")
		}
		route.Upstream = strings.TrimSpace(route.Upstream)
		route.Network = strings.TrimSpace(route.Network)
		route.TrustFile = strings.TrimSpace(route.TrustFile)
		route.ServerName = strings.TrimSpace(route.ServerName)
		if route.Key != "" {
			byKey[route.Key] = route
		}
	}
	out := make([]Route, 0, len(byKey))
	for _, route := range byKey {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if len(out[i].PathPrefix) != len(out[j].PathPrefix) {
			return len(out[i].PathPrefix) > len(out[j].PathPrefix)
		}
		return out[i].Key < out[j].Key
	})
	return out
}
