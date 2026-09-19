package exposure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/endpoint"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	stateVersion = 1
	caddyImage   = "caddy:2.11.4-alpine"
)

type Deployment struct {
	Hostname string
	TLSMode  string
	TLSDir   string
}

type Route struct {
	Name           string `json:"name"`
	Service        string `json:"service"`
	TargetPort     int    `json:"target_port"`
	Protocol       string `json:"protocol"`
	Visibility     string `json:"visibility"`
	PublishedPort  int    `json:"published_port"`
	TLSFingerprint string `json:"tls_fingerprint,omitempty"`
}

type State struct {
	Version int     `json:"version"`
	Project string  `json:"project"`
	Network string  `json:"network"`
	Host    string  `json:"host"`
	Routes  []Route `json:"routes"`
}

type Files struct {
	Dir     string
	Compose string
	Env     string
	State   string
}

type Driver struct {
	compose    bhruntime.Compose
	manifest   application.Manifest
	runtime    application.RuntimeFiles
	deployment Deployment

	files         Files
	state         State
	wasRunning    bool
	provisioned   bool
	changed       bool
	previousFiles map[string][]byte
	planned       map[string]capability.HTTPExposureBinding
}

func ProjectName(m application.Manifest) string {
	return "baseharbor-exposure-" + m.Name + "-" + m.Environment
}

func FilesFor(runtime application.RuntimeFiles) Files {
	dir := filepath.Join(runtime.Dir, "providers", "caddy")
	return Files{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "provider.env"),
		State:   filepath.Join(dir, "state.json"),
	}
}

func NewDriver(compose bhruntime.Compose, m application.Manifest, runtime application.RuntimeFiles, deployment Deployment) *Driver {
	return &Driver{compose: compose, manifest: m, runtime: runtime, deployment: deployment, files: FilesFor(runtime)}
}

func (d *Driver) Descriptor() capability.Provider { return capability.Caddy }

func (d *Driver) IntegrationDescriptor() capability.IntegrationDescriptor {
	return capability.CaddyIntegration
}

func (d *Driver) Preflight(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.ExposureHTTP {
		return fmt.Errorf("Caddy cannot preflight capability %q", resource.Kind)
	}
	if strings.TrimSpace(d.deployment.Hostname) == "" {
		return errors.New("managed HTTP exposure requires repository deployment initialization; run 'baha app init'")
	}
	if binding.HTTPExposure == nil {
		return fmt.Errorf("HTTP exposure %q binding metadata is required", resource.Name)
	}
	route := *binding.HTTPExposure
	if strings.TrimSpace(route.Service) == "" || route.TargetPort < 1 || route.TargetPort > 65535 {
		return fmt.Errorf("HTTP exposure %q has invalid logical endpoint metadata", resource.Name)
	}
	if route.Protocol != "http" && route.Protocol != "https" {
		return fmt.Errorf("HTTP exposure %q protocol %q is unsupported", resource.Name, route.Protocol)
	}
	if route.Visibility != "public" && route.Visibility != "internal" {
		return fmt.Errorf("HTTP exposure %q visibility %q is unsupported", resource.Name, route.Visibility)
	}
	if binding.Workload != "service/"+route.Service {
		return fmt.Errorf("HTTP exposure %q binding targets %q, expected service/%s", resource.Name, binding.Workload, route.Service)
	}
	if route.Protocol == "https" {
		if d.deployment.TLSMode != "existing" {
			return fmt.Errorf("managed HTTPS exposure %q currently requires existing/BYOC TLS; deployment TLS mode is %q", resource.Name, d.deployment.TLSMode)
		}
		for _, name := range []string{"cert.pem", "key.pem"} {
			path := filepath.Join(d.deployment.TLSDir, name)
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("managed HTTPS exposure %q requires %s: %w", resource.Name, path, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("managed HTTPS exposure %q TLS path %s is not a regular file", resource.Name, path)
			}
		}
	}
	if err := capability.RequireIntegrationContract(d.IntegrationDescriptor()); err != nil {
		return err
	}
	if d.planned == nil {
		d.planned = make(map[string]capability.HTTPExposureBinding)
	}
	if previous, exists := d.planned[resource.Name]; exists && previous != route {
		return fmt.Errorf("HTTP exposure %q has conflicting preflight bindings", resource.Name)
	}
	d.planned[resource.Name] = route
	return nil
}

func (d *Driver) Provision(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	if d.provisioned {
		return nil
	}
	if snapshot, err := snapshotDirectory(d.files.Dir); err != nil {
		return fmt.Errorf("snapshot existing Caddy provider state: %w", err)
	} else {
		d.previousFiles = snapshot
	}
	if _, err := os.Stat(d.files.Compose); err == nil {
		running, runErr := d.compose.RunningServicesProject(ctx, ProjectName(d.manifest), d.files.Compose, d.files.Env)
		if runErr != nil {
			return fmt.Errorf("inspect existing Caddy exposure provider: %w", runErr)
		}
		d.wasRunning = len(running) > 0
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	state, changed, err := d.ensureFiles()
	if err != nil {
		_ = d.restorePreviousFiles()
		return err
	}
	d.state = state
	d.changed = changed
	if err := d.compose.ConfigProject(ctx, state.Project, d.files.Compose, d.files.Env); err != nil {
		_ = d.restorePreviousFiles()
		return fmt.Errorf("validate Caddy exposure provider: %w", err)
	}
	if changed && d.wasRunning {
		if err := d.compose.DownProjectRemoveOrphans(ctx, state.Project, d.files.Compose, d.files.Env); err != nil {
			_ = d.Rollback(context.WithoutCancel(ctx))
			return fmt.Errorf("restart changed Caddy exposure provider: %w", err)
		}
	}
	if err := d.compose.UpProject(ctx, state.Project, d.files.Compose, d.files.Env); err != nil {
		_ = d.Rollback(context.WithoutCancel(ctx))
		return fmt.Errorf("start Caddy exposure provider: %w", err)
	}
	d.provisioned = true
	return nil
}

func (d *Driver) restorePreviousFiles() error {
	if err := os.RemoveAll(d.files.Dir); err != nil {
		return err
	}
	if len(d.previousFiles) == 0 {
		return nil
	}
	return restoreDirectory(d.files.Dir, d.previousFiles)
}

func (d *Driver) Rollback(ctx context.Context) error {
	if !d.changed && d.wasRunning {
		return nil
	}
	var result error
	if _, err := os.Stat(d.files.Compose); err == nil {
		if err := d.compose.DownProjectRemoveOrphans(ctx, ProjectName(d.manifest), d.files.Compose, d.files.Env); err != nil {
			result = errors.Join(result, err)
		}
	}
	if err := d.restorePreviousFiles(); err != nil {
		return errors.Join(result, err)
	}
	if len(d.previousFiles) > 0 {
		if d.wasRunning {
			if err := d.compose.UpProject(ctx, ProjectName(d.manifest), d.files.Compose, d.files.Env); err != nil {
				result = errors.Join(result, fmt.Errorf("restore previous Caddy exposure provider: %w", err))
			}
		}
	}
	d.provisioned = false
	return result
}

func (d *Driver) Bind(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	route, ok := d.stateRoute(resource.Name)
	if !ok {
		return fmt.Errorf("Caddy exposure %q was not materialized", resource.Name)
	}
	if binding.Workload != "service/"+route.Service {
		return fmt.Errorf("Caddy exposure %q has invalid workload binding %q", resource.Name, binding.Workload)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	route, ok := d.stateRoute(resource.Name)
	if !ok {
		return fmt.Errorf("Caddy exposure %q state is missing", resource.Name)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status := endpoint.ProbeHTTPDialTarget(probeCtx, endpoint.Endpoint{
		Service: route.Service,
		Scheme:  route.Protocol,
		Host:    d.state.Host,
		Port:    route.PublishedPort,
	}, "127.0.0.1", route.PublishedPort)
	if !status.Ready {
		_ = d.Rollback(context.WithoutCancel(ctx))
		return fmt.Errorf("managed exposure %s://%s:%d is not ready: %s", route.Protocol, d.state.Host, route.PublishedPort, status.Detail)
	}
	return nil
}

func (d *Driver) State() State { return d.state }

func Load(runtime application.RuntimeFiles) (State, Files, error) {
	files := FilesFor(runtime)
	data, err := os.ReadFile(files.State)
	if err != nil {
		return State{}, files, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, files, fmt.Errorf("decode Caddy exposure state: %w", err)
	}
	if state.Version != stateVersion {
		return State{}, files, fmt.Errorf("unsupported Caddy exposure state version %d", state.Version)
	}
	return state, files, nil
}

func Inspect(ctx context.Context, compose bhruntime.Compose, runtime application.RuntimeFiles) (State, []endpoint.ExposureStatus, error) {
	state, files, err := Load(runtime)
	if err != nil {
		return State{}, nil, err
	}
	running, err := compose.RunningServicesProject(ctx, state.Project, files.Compose, files.Env)
	if err != nil {
		return state, nil, err
	}
	if len(running) == 0 {
		return state, nil, errors.New("managed Caddy exposure provider is stopped")
	}
	statuses := make([]endpoint.ExposureStatus, 0, len(state.Routes))
	for _, route := range state.Routes {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		status := endpoint.ProbeHTTPDialTarget(probeCtx, endpoint.Endpoint{
			Service: route.Service, Scheme: route.Protocol, Host: state.Host, Port: route.PublishedPort,
		}, "127.0.0.1", route.PublishedPort)
		cancel()
		statuses = append(statuses, status)
	}
	endpoint.SortExposureStatuses(statuses)
	for _, status := range statuses {
		if !status.Ready {
			return state, statuses, fmt.Errorf("managed exposure %s://%s:%d is not ready: %s", status.Scheme, status.Host, status.Port, status.Detail)
		}
	}
	return state, statuses, nil
}

func Stop(ctx context.Context, compose bhruntime.Compose, runtime application.RuntimeFiles) error {
	state, files, err := Load(runtime)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return compose.DownProjectRemoveOrphans(ctx, state.Project, files.Compose, files.Env)
}

func Destroy(ctx context.Context, compose bhruntime.Compose, runtime application.RuntimeFiles) error {
	state, files, err := Load(runtime)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := compose.DestroyProjectRemoveOrphans(ctx, state.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func (d *Driver) stateRoute(name string) (Route, bool) {
	for _, route := range d.state.Routes {
		if route.Name == name {
			return route, true
		}
	}
	return Route{}, false
}

func (d *Driver) ensureFiles() (State, bool, error) {
	if err := os.MkdirAll(d.files.Dir, 0o700); err != nil {
		return State{}, false, fmt.Errorf("create Caddy provider state: %w", err)
	}
	if err := os.Chmod(d.files.Dir, 0o700); err != nil {
		return State{}, false, err
	}
	old, _, oldErr := Load(d.runtime)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return State{}, false, oldErr
	}
	previous := map[string]Route{}
	if oldErr == nil {
		for _, route := range old.Routes {
			previous[route.Name] = route
		}
	}
	used := map[int]struct{}{}
	var routes []Route
	plannedNames := make([]string, 0, len(d.planned))
	for name := range d.planned {
		plannedNames = append(plannedNames, name)
	}
	sort.Strings(plannedNames)
	for _, name := range plannedNames {
		requirement := d.planned[name]
		route := Route{Name: name, Service: requirement.Service, TargetPort: requirement.TargetPort, Protocol: requirement.Protocol, Visibility: requirement.Visibility}
		if route.Protocol == "https" {
			certData, err := os.ReadFile(filepath.Join(d.deployment.TLSDir, "cert.pem"))
			if err != nil {
				return State{}, false, err
			}
			sum := sha256.Sum256(certData)
			route.TLSFingerprint = fmt.Sprintf("%x", sum[:])
		}
		if prior, ok := previous[route.Name]; ok && prior.Service == route.Service && prior.TargetPort == route.TargetPort && prior.Protocol == route.Protocol && prior.Visibility == route.Visibility && prior.PublishedPort > 0 {
			route.PublishedPort = prior.PublishedPort
		} else {
			preferred := 8080
			if route.Protocol == "https" {
				preferred = 8443
			}
			port, err := choosePublishedPort(preferred, route.Visibility, used)
			if err != nil {
				return State{}, false, err
			}
			route.PublishedPort = port
		}
		used[route.PublishedPort] = struct{}{}
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
	state := State{
		Version: stateVersion,
		Project: ProjectName(d.manifest),
		Network: application.ApplicationExposureNetworkName(d.manifest),
		Host:    d.deployment.Hostname,
		Routes:  routes,
	}
	changed := oldErr != nil || !sameState(old, state)

	for _, route := range routes {
		routeDir := filepath.Join(d.files.Dir, "routes", route.Name)
		if err := os.MkdirAll(routeDir, 0o700); err != nil {
			return State{}, false, err
		}
		if err := writeOwnerOnly(filepath.Join(routeDir, "Caddyfile"), []byte(caddyfile(route))); err != nil {
			return State{}, false, err
		}
		if route.Protocol == "https" {
			for _, name := range []string{"cert.pem", "key.pem"} {
				data, err := os.ReadFile(filepath.Join(d.deployment.TLSDir, name))
				if err != nil {
					return State{}, false, err
				}
				if err := writeOwnerOnly(filepath.Join(routeDir, name), data); err != nil {
					return State{}, false, err
				}
			}
		}
	}
	if err := writeOwnerOnly(d.files.Env, []byte("# BaseHarbor managed Caddy exposure provider\n")); err != nil {
		return State{}, false, err
	}
	if err := writeOwnerOnly(d.files.Compose, []byte(composeYAML(state, d.files))); err != nil {
		return State{}, false, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return State{}, false, err
	}
	data = append(data, '\n')
	if err := writeOwnerOnly(d.files.State, data); err != nil {
		return State{}, false, err
	}
	return state, changed, nil
}

func composeYAML(state State, files Files) string {
	var b strings.Builder
	b.WriteString("services:\n")
	for _, route := range state.Routes {
		serviceName := "route-" + route.Name
		containerPort := 80
		if route.Protocol == "https" {
			containerPort = 443
		}
		fmt.Fprintf(&b, "  %s:\n", serviceName)
		fmt.Fprintf(&b, "    image: %s\n", caddyImage)
		b.WriteString("    restart: unless-stopped\n")
		if route.Visibility == "internal" {
			fmt.Fprintf(&b, "    ports:\n      - \"127.0.0.1:%d:%d\"\n", route.PublishedPort, containerPort)
		} else {
			fmt.Fprintf(&b, "    ports:\n      - \"%d:%d\"\n", route.PublishedPort, containerPort)
		}
		b.WriteString("    volumes:\n")
		routeDir := filepath.Join(files.Dir, "routes", route.Name)
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(filepath.Join(routeDir, "Caddyfile")+":/etc/caddy/Caddyfile:ro"))
		if route.Protocol == "https" {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(filepath.Join(routeDir, "cert.pem")+":/certs/cert.pem:ro"))
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(filepath.Join(routeDir, "key.pem")+":/certs/key.pem:ro"))
		}
		b.WriteString("    networks:\n      application: {}\n")
	}
	b.WriteString("networks:\n  application:\n    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", state.Network)
	return b.String()
}

func caddyfile(route Route) string {
	listen := ":80"
	var tlsLine string
	if route.Protocol == "https" {
		listen = ":443"
		tlsLine = "  tls /certs/cert.pem /certs/key.pem\n"
	}
	return fmt.Sprintf("%s {\n%s  reverse_proxy %s:%d\n}\n", listen, tlsLine, route.Service, route.TargetPort)
}

func choosePublishedPort(preferred int, visibility string, used map[int]struct{}) (int, error) {
	bindHost := "0.0.0.0"
	if visibility == "internal" {
		bindHost = "127.0.0.1"
	}
	if _, taken := used[preferred]; !taken {
		ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, strconv.Itoa(preferred)))
		if err == nil {
			_ = ln.Close()
			return preferred, nil
		}
	}
	for attempt := 0; attempt < 32; attempt++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
		if err != nil {
			return 0, fmt.Errorf("allocate Caddy exposure host port: %w", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		if _, taken := used[port]; !taken {
			return port, nil
		}
	}
	return 0, errors.New("allocate Caddy exposure host port: exhausted retries")
}

func writeOwnerOnly(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func snapshotDirectory(root string) (map[string][]byte, error) {
	result := map[string][]byte{}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Caddy provider state path %s is not a directory", root)
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Caddy provider state contains non-regular file %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = data
		return nil
	})
	return result, err
}

func restoreDirectory(root string, files map[string][]byte) error {
	for rel, data := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := writeOwnerOnly(path, data); err != nil {
			return err
		}
	}
	return nil
}

func sameState(a, b State) bool {
	if a.Version != b.Version || a.Project != b.Project || a.Network != b.Network || a.Host != b.Host || len(a.Routes) != len(b.Routes) {
		return false
	}
	for i := range a.Routes {
		if a.Routes[i] != b.Routes[i] {
			return false
		}
	}
	return true
}
