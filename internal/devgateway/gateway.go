package devgateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	Owner      string `json:"owner"`
	Key        string `json:"key"`
	Host       string `json:"host"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Upstream   string `json:"upstream"`
	Network    string `json:"network"`
	TrustFile  string `json:"trust_file"`
	ServerName string `json:"server_name"`
}

type OwnerRoutes struct {
	Owner  string
	Routes []Route
}

type state struct {
	Version  int     `json:"version"`
	HostPort int     `json:"host_port,omitempty"`
	Routes   []Route `json:"routes"`
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
		Caddyfile: filepath.Join(dir, "Caddyfile"),
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
	return Reconcile(ctx, runtime, issuer, target)
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
	if len(next.Routes) > 0 && routeNetworkSetChanged(previous.Routes, next.Routes) {
		if _, err := os.Stat(files.Compose); err == nil {
			if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
				return fmt.Errorf("restart development gateway after route network change: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
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
	if runtime == nil {
		return errors.New("development gateway runtime is required")
	}
	if issuer == nil {
		return errors.New("development gateway issuer is required")
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
		if _, statErr := os.Stat(files.Compose); statErr == nil {
			if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
				return fmt.Errorf("stop empty development gateway: %w", err)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		for _, path := range []string{files.Compose, files.Env, files.Caddyfile} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Join(files.Dir, "runtime", "trust"), 0o700); err != nil {
		return err
	}
	hosts := make([]string, 0, len(current.Routes))
	for _, route := range current.Routes {
		hosts = append(hosts, route.Host)
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
	if err := projectReadable(material.ServerKey, files.Key); err != nil {
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
	hostPort, err := resolveGatewayHostPort(files, current, runtime)
	if err != nil {
		return err
	}
	current.HostPort = hostPort
	if err := os.WriteFile(files.Caddyfile, []byte(renderCaddyfile(current.Routes, hostPort)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(files.Env, []byte(""), 0o600); err != nil {
		return err
	}
	if err := saveState(files.State, current); err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(renderCompose(files, current.Routes, trustTargets, hostPort)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate development gateway: %w", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start development gateway on local HTTPS port %d: %w", hostPort, err)
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
	}
	for host := range wanted {
		if _, ok := seen[host]; !ok {
			errs = append(errs, fmt.Errorf("%s is not registered in the development gateway", host))
		}
	}
	return errors.Join(errs...)
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
		return fmt.Errorf("%s: %w", route.Host, err)
	}
	return nil
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
	data, err := os.ReadFile(path)
	if err != nil {
		return state{}, err
	}
	var value state
	if err := json.Unmarshal(data, &value); err != nil {
		return state{}, fmt.Errorf("decode development gateway routes: %w", err)
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
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
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

func projectReadable(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(source))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
