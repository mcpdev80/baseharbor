package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	providerProject      = "baseharbor-logs"
	workloadOverrideName = "workload.logging.override.yaml"
	providerOverrideName = "provider.logging.override.yaml"
)

type Placement struct {
	Scope            capability.ProviderScope
	Project          string
	Network          string
	Dir              string
	LokiVolume       string
	AlloyVolume      string
	SharingBoundary  string
	OwnerApplication string
}

type Registration struct {
	Application        string `json:"application"`
	Environment        string `json:"environment"`
	SyslogPort         int    `json:"syslog_port"`
	ProviderSyslogPort int    `json:"provider_syslog_port"`
}

type ProviderFiles struct {
	Dir           string
	Compose       string
	Env           string
	LokiConfig    string
	AlloyConfig   string
	Registrations string
}

func PlacementFor(m application.Manifest) (Placement, error) {
	p, err := application.ResolveProviderPlacement(m, capability.ProviderLoki)
	if err != nil {
		return Placement{}, err
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	switch p.Scope {
	case capability.ScopeShared:
		project := providerProject
		dir := filepath.Join(dataDir, "providers", "loki", "shared")
		lokiVolume := "baseharbor-loki-data"
		alloyVolume := "baseharbor-alloy-data"
		if p.SharingBoundary != "" {
			token := application.ProviderPlacementNameToken(p.SharingBoundary)
			project += "-" + token
			dir = filepath.Join(dir, token)
			lokiVolume += "-" + token
			alloyVolume += "-" + token
		}
		network := project + "-internal"
		return Placement{Scope: p.Scope, Project: project, Network: network, Dir: dir, LokiVolume: lokiVolume, AlloyVolume: alloyVolume, SharingBoundary: p.SharingBoundary}, nil
	case capability.ScopeApplication:
		suffix := m.Name + "-" + m.Environment
		project := providerProject + "-" + suffix
		return Placement{
			Scope:            p.Scope,
			Project:          project,
			Network:          project + "-internal",
			Dir:              filepath.Join(dataDir, "providers", "loki", "applications", m.Name, m.Environment),
			LokiVolume:       "baseharbor-loki-data-" + suffix,
			AlloyVolume:      "baseharbor-alloy-data-" + suffix,
			OwnerApplication: m.Name,
		}, nil
	case capability.ScopeExternal:
		return Placement{Scope: p.Scope}, nil
	default:
		return Placement{}, fmt.Errorf("unsupported Loki provider scope %q", p.Scope)
	}
}

func providerFiles(p Placement) ProviderFiles {
	return ProviderFiles{
		Dir:           p.Dir,
		Compose:       filepath.Join(p.Dir, "compose.yaml"),
		Env:           filepath.Join(p.Dir, "runtime.env"),
		LokiConfig:    filepath.Join(p.Dir, "loki.yaml"),
		AlloyConfig:   filepath.Join(p.Dir, "config.alloy"),
		Registrations: filepath.Join(p.Dir, "registrations.json"),
	}
}

func EnsureProviderFiles(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest) (ProviderFiles, error) {
	return EnsureProviderFilesForRuntime(ctx, issuer, m, "docker")
}

func EnsureProviderFilesForRuntime(ctx context.Context, issuer serviceaccess.Issuer, m application.Manifest, runtimeKind string) (ProviderFiles, error) {
	p, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external Loki provider has no BaseHarbor-owned provider files")
	}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(p.Dir, 0o700); err != nil {
		return ProviderFiles{}, err
	}
	files := providerFiles(p)
	registrations, err := reconcileRegistration(files.Registrations, m, true)
	if err != nil {
		return ProviderFiles{}, err
	}
	providerSources, err := providerLogSources(p, registrations)
	if err != nil {
		return ProviderFiles{}, err
	}
	lokiPort, err := persistedOrAllocatedPort(files.Env, "BASEHARBOR_LOKI_PORT")
	if err != nil {
		return ProviderFiles{}, err
	}
	platformSyslogPort := 0
	if strings.EqualFold(strings.TrimSpace(runtimeKind), "docker") && hasPlatformProviderLogs(providerSources) {
		platformSyslogPort, err = persistedOrAllocatedUDPPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
		if err != nil {
			return ProviderFiles{}, err
		}
	}
	var env strings.Builder
	fmt.Fprintf(&env, "BASEHARBOR_LOKI_PORT=%d\n", lokiPort)
	if platformSyslogPort > 0 {
		fmt.Fprintf(&env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT=%d\n", platformSyslogPort)
	}
	if err := os.WriteFile(files.Env, []byte(env.String()), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.LokiConfig, []byte(lokiConfig()), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.LokiConfig, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.AlloyConfig, []byte(alloyConfigForRuntimeSources(registrations, providerSources, runtimeKind, platformSyslogPort)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.AlloyConfig, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy, err := serviceaccess.Resolve(lokiAccessEnvironment(m, registrations), "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, lokiAccessSpec())
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLForRuntimeAndAccess(p, registrations, runtimeKind, accessFiles, platformSyslogPort)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles(m application.Manifest) (ProviderFiles, error) {
	p, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external Loki provider has no BaseHarbor-owned provider files")
	}
	files := providerFiles(p)
	for _, path := range []string{files.Compose, files.Env, files.LokiConfig, files.AlloyConfig, files.Registrations} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func ApplicationRegistration(m application.Manifest) (Registration, error) {
	files, err := ExistingProviderFiles(m)
	if err != nil {
		return Registration{}, err
	}
	registrations, err := readRegistrations(files.Registrations)
	if err != nil {
		return Registration{}, err
	}
	for _, r := range registrations {
		if r.Application == m.Name && r.Environment == m.Environment {
			return r, nil
		}
	}
	return Registration{}, fmt.Errorf("Loki collector registration for %s/%s is missing", m.Name, m.Environment)
}

func EnsureWorkloadOverride(m application.Manifest, runtime application.RuntimeFiles, services []string) (string, error) {
	return EnsureWorkloadOverrideForRuntime(m, runtime, services, "docker")
}

func EnsureWorkloadOverrideForRuntime(m application.Manifest, runtime application.RuntimeFiles, services []string, runtimeKind string) (string, error) {
	registration, err := ApplicationRegistration(m)
	if err != nil {
		return "", err
	}
	if len(services) == 0 {
		return "", errors.New("at least one workload service is required for log collection")
	}
	path := filepath.Join(runtime.Dir, workloadOverrideName)
	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n", service)
		b.WriteString("    logging:\n")
		if strings.EqualFold(strings.TrimSpace(runtimeKind), "podman") {
			b.WriteString("      driver: journald\n")
			continue
		}
		b.WriteString("      driver: syslog\n")
		b.WriteString("      options:\n")
		fmt.Fprintf(&b, "        syslog-address: %s\n", strconv.Quote(fmt.Sprintf("udp://127.0.0.1:%d", registration.SyslogPort)))
		b.WriteString("        syslog-format: rfc5424\n")
		fmt.Fprintf(&b, "        tag: %s\n", strconv.Quote(service))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func EnsureProviderSourceOverrideForRuntime(m application.Manifest, runtime application.RuntimeFiles, runtimeKind string) (string, bool, error) {
	return EnsureRuntimeProjectOverrideForRuntime(
		m,
		runtime.Dir,
		providerOverrideName,
		application.RuntimeProjectName(m),
		runtimeKind,
		observability.SourceApplicationProvider,
	)
}

func EnsureRuntimeProjectOverrideForRuntime(
	m application.Manifest,
	dir string,
	filename string,
	project string,
	runtimeKind string,
	class observability.SourceClass,
) (string, bool, error) {
	p, err := PlacementFor(m)
	if err != nil {
		return "", false, err
	}
	policy, err := application.LogsPolicy(m)
	if err != nil {
		return "", false, err
	}
	includeApplicationProviders := policy.Enabled && policy.Collect[application.LogsSourceApplicationProvider]
	includePlatformProviders := policy.Enabled && policy.Collect[application.LogsSourcePlatformProvider]
	sources, err := observability.ListLogs(
		capability.ProviderPlacement{Scope: p.Scope, SharingBoundary: p.SharingBoundary, Ownership: capability.OwnershipBaseHarbor},
		[]string{m.Name},
		includeApplicationProviders,
		includePlatformProviders,
	)
	if err != nil {
		return "", false, err
	}

	type providerService struct {
		Service  string
		Provider capability.ProviderKind
		Class    observability.SourceClass
	}
	seen := map[string]providerService{}
	for _, source := range sources {
		if source.Class != class {
			continue
		}
		sourceProject, service, ok := observability.ParseRuntimeTarget(source.Target)
		if !ok || sourceProject != project {
			continue
		}
		seen[service] = providerService{Service: service, Provider: source.Provider, Class: source.Class}
	}

	path := filepath.Join(dir, filename)
	if len(seen) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		return "", false, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}

	port := 0
	if !strings.EqualFold(strings.TrimSpace(runtimeKind), "podman") {
		switch class {
		case observability.SourceApplicationProvider:
			registration, err := ApplicationRegistration(m)
			if err != nil {
				return "", false, err
			}
			port = registration.ProviderSyslogPort
		case observability.SourcePlatformProvider:
			files, err := ExistingProviderFiles(m)
			if err != nil {
				return "", false, err
			}
			port, err = persistedPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
			if err != nil {
				return "", false, err
			}
		default:
			return "", false, fmt.Errorf("unsupported runtime log source class %q", class)
		}
	}

	services := make([]providerService, 0, len(seen))
	for _, service := range seen {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Service < services[j].Service })

	var b strings.Builder
	b.WriteString("services:\n")
	for _, source := range services {
		fmt.Fprintf(&b, "  %s:\n", source.Service)
		b.WriteString("    logging:\n")
		if strings.EqualFold(strings.TrimSpace(runtimeKind), "podman") {
			b.WriteString("      driver: journald\n")
			continue
		}
		b.WriteString("      driver: syslog\n")
		b.WriteString("      options:\n")
		fmt.Fprintf(&b, "        syslog-address: %s\n", strconv.Quote(fmt.Sprintf("udp://127.0.0.1:%d", port)))
		b.WriteString("        syslog-format: rfc5424\n")
		fmt.Fprintf(&b, "        tag: %s\n", strconv.Quote(string(source.Provider)+"/"+source.Service))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func ExistingProviderSourceOverride(runtime application.RuntimeFiles) (string, bool, error) {
	path := filepath.Join(runtime.Dir, providerOverrideName)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, errors.New("provider logging override is not a regular file")
	}
	return path, true, nil
}

func ExistingWorkloadOverride(runtime application.RuntimeFiles) (string, bool, error) {
	path := filepath.Join(runtime.Dir, workloadOverrideName)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, errors.New("logging workload override is not a regular file")
	}
	return path, true, nil
}

func RemoveProviderSourceOverride(runtime application.RuntimeFiles) error {
	path := filepath.Join(runtime.Dir, providerOverrideName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func RemoveWorkloadOverride(runtime application.RuntimeFiles) error {
	path := filepath.Join(runtime.Dir, workloadOverrideName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func UnregisterApplication(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, m application.Manifest) error {
	p, err := PlacementFor(m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return err
	}
	files := providerFiles(p)
	existing, err := readRegistrations(files.Registrations)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	registered := false
	for _, registration := range existing {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			registered = true
			break
		}
	}
	if !registered {
		return nil
	}
	registrations, err := reconcileRegistration(files.Registrations, m, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.Scope == capability.ScopeApplication || len(registrations) == 0 {
		return DestroyProvider(ctx, runtime, m)
	}
	providerSources, err := providerLogSources(p, registrations)
	if err != nil {
		return err
	}
	platformSyslogPort := 0
	if runtimeKind(runtime) == "docker" && hasPlatformProviderLogs(providerSources) {
		platformSyslogPort, err = persistedOrAllocatedUDPPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(files.AlloyConfig, []byte(alloyConfigForRuntimeSources(registrations, providerSources, runtimeKind(runtime), platformSyslogPort)), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(files.AlloyConfig, 0o644); err != nil {
		return err
	}
	accessPolicy, err := serviceaccess.Resolve(lokiAccessEnvironment(m, registrations), "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, lokiAccessSpec())
	if err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLForRuntimeAndAccess(p, registrations, runtimeKind(runtime), accessFiles, platformSyslogPort)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return runtime.UpProject(ctx, p.Project, files.Compose, files.Env)
}

func StopProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	p, err := PlacementFor(m)
	if err != nil || p.Scope != capability.ScopeApplication {
		return err
	}
	files, err := ExistingProviderFiles(m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return runtime.StopProject(ctx, p.Project, files.Compose, files.Env)
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	p, err := PlacementFor(m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return err
	}
	files := providerFiles(p)
	if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return err
	}
	_ = observability.Remove("loki:" + p.Project)
	return os.RemoveAll(p.Dir)
}

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_LOKI_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Loki API port")
			}
			return fmt.Sprintf("https://127.0.0.1:%d", port), nil
		}
	}
	return "", errors.New("Loki API port is missing")
}

func lokiAccessEnvironment(m application.Manifest, registrations []Registration) string {
	managed := !lokiDevelopmentEnvironment(m.Environment)
	for _, registration := range registrations {
		if !lokiDevelopmentEnvironment(registration.Environment) {
			managed = true
			break
		}
	}
	if managed {
		return "prod"
	}
	return "dev"
}

func lokiDevelopmentEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true
	default:
		return false
	}
}

func lokiAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "loki-access",
		Upstream:         "http://loki:3100",
		PublishedPortEnv: "BASEHARBOR_LOKI_PORT",
		ContainerPort:    8443,
		Networks:         []string{"logs-internal", "logs-publish"},
		RequireClient:    true,
	}
}

func providerLogSources(p Placement, registrations []Registration) ([]observability.SignalSource, error) {
	applications := make([]string, 0, len(registrations))
	seen := map[string]struct{}{}
	for _, registration := range registrations {
		if _, ok := seen[registration.Application]; ok {
			continue
		}
		seen[registration.Application] = struct{}{}
		applications = append(applications, registration.Application)
	}
	return observability.ListLogs(
		capability.ProviderPlacement{
			Scope:           p.Scope,
			SharingBoundary: p.SharingBoundary,
			Ownership:       capability.OwnershipBaseHarbor,
		},
		applications,
		true,
		true,
	)
}

func reconcileRegistration(path string, m application.Manifest, present bool) ([]Registration, error) {
	registrations, err := readRegistrations(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	result := make([]Registration, 0, len(registrations)+1)
	var existing *Registration
	for _, registration := range registrations {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			copy := registration
			existing = &copy
			continue
		}
		result = append(result, registration)
	}
	if present {
		r := Registration{Application: m.Name, Environment: m.Environment}
		if existing != nil {
			r.SyslogPort = existing.SyslogPort
			r.ProviderSyslogPort = existing.ProviderSyslogPort
		}
		if r.SyslogPort == 0 {
			r.SyslogPort, err = allocatePort("udp")
			if err != nil {
				return nil, err
			}
		}
		if r.ProviderSyslogPort == 0 {
			r.ProviderSyslogPort, err = allocatePort("udp")
			if err != nil {
				return nil, err
			}
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Application == result[j].Application {
			return result[i].Environment < result[j].Environment
		}
		return result[i].Application < result[j].Application
	})
	if !present && errors.Is(err, os.ErrNotExist) {
		return result, os.ErrNotExist
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return result, nil
}

func readRegistrations(path string) ([]Registration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registrations []Registration
	if err := json.Unmarshal(data, &registrations); err != nil {
		return nil, fmt.Errorf("decode Loki registrations: %w", err)
	}
	for _, r := range registrations {
		if strings.TrimSpace(r.Application) == "" || strings.TrimSpace(r.Environment) == "" ||
			r.SyslogPort < 1 || r.SyslogPort > 65535 ||
			r.ProviderSyslogPort < 1 || r.ProviderSyslogPort > 65535 {
			return nil, errors.New("invalid Loki registration state")
		}
	}
	return registrations, nil
}

func hasPlatformProviderLogs(sources []observability.SignalSource) bool {
	for _, source := range sources {
		if source.Kind == observability.SignalLogs && source.Class == observability.SourcePlatformProvider {
			return true
		}
	}
	return false
}

func persistedPort(path, key string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			port, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil && port > 0 && port <= 65535 {
				return port, nil
			}
			return 0, fmt.Errorf("invalid %s port", key)
		}
	}
	return 0, fmt.Errorf("%s is not materialized", key)
}

func persistedOrAllocatedUDPPort(path, key string) (int, error) {
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && k == key {
				port, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil && port > 0 && port <= 65535 {
					return port, nil
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return allocatePort("udp")
}

func persistedOrAllocatedPort(path, key string) (int, error) {
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && k == key {
				port, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil && port > 0 && port <= 65535 {
					return port, nil
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return allocatePort("tcp")
}

func allocatePort(network string) (int, error) {
	if network == "tcp" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port, nil
	}
	if network == "udp" {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		if err != nil {
			return 0, err
		}
		defer listener.Close()
		return listener.LocalAddr().(*net.UDPAddr).Port, nil
	}
	return 0, fmt.Errorf("unsupported port allocation network %q", network)
}
