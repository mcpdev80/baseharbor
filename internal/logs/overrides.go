package logs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func ApplicationRegistration(m application.Manifest) (Registration, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Registration{}, err
	}
	return ApplicationRegistrationAt(dataDir, "", m)
}

func ApplicationRegistrationAt(dataDir, namespace string, m application.Manifest) (Registration, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
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
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", err
	}
	return EnsureWorkloadOverrideForRuntimeAt(dataDir, "", m, runtime, services, runtimeKind)
}

func EnsureWorkloadOverrideForRuntimeAt(dataDir, namespace string, m application.Manifest, runtime application.RuntimeFiles, services []string, runtimeKind string) (string, error) {
	registration, err := ApplicationRegistrationAt(dataDir, namespace, m)
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
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", false, err
	}
	return EnsureProviderSourceOverrideForRuntimeAt(dataDir, "", m, runtime, runtimeKind)
}

func EnsureProviderSourceOverrideForRuntimeAt(dataDir, namespace string, m application.Manifest, runtime application.RuntimeFiles, runtimeKind string) (string, bool, error) {
	project := strings.TrimSpace(runtime.Project)
	if project == "" {
		project = application.RuntimeProjectName(m)
	}
	return EnsureRuntimeProjectOverrideForRuntimeAt(
		dataDir,
		namespace,
		m,
		runtime.Dir,
		providerOverrideName,
		project,
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
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", false, err
	}
	return EnsureRuntimeProjectOverrideForRuntimeAt(dataDir, "", m, dir, filename, project, runtimeKind, class)
}

func EnsureRuntimeProjectOverrideForRuntimeAt(
	dataDir string,
	namespace string,
	m application.Manifest,
	dir string,
	filename string,
	project string,
	runtimeKind string,
	class observability.SourceClass,
) (string, bool, error) {
	p, err := PlacementForAt(dataDir, namespace, m)
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
			registration, err := ApplicationRegistrationAt(dataDir, namespace, m)
			if err != nil {
				return "", false, err
			}
			port = registration.ProviderSyslogPort
		case observability.SourcePlatformProvider:
			files, err := ExistingProviderFilesAt(dataDir, namespace, m)
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
