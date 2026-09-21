package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	ProviderProject = "baseharbor-metrics"
	ProviderService = "prometheus"
	ProviderImage   = "docker.io/prom/prometheus:v3.14.0"
)

type Placement struct {
	Scope   capability.ProviderScope
	Project string
	Network string
	Volume  string
	Dir     string
}

func PlacementFor(m application.Manifest) (Placement, error) {
	providerPlacement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return Placement{}, err
	}
	return placementFromProviderPlacement(m, providerPlacement)
}

func RegisteredPlacementFor(m application.Manifest) (Placement, bool, error) {
	providerPlacement, found, err := application.RegisteredProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil || !found {
		return Placement{}, found, err
	}
	placement, err := placementFromProviderPlacement(m, providerPlacement)
	if err != nil {
		return Placement{}, false, err
	}
	return placement, true, nil
}

func placementFromProviderPlacement(m application.Manifest, providerPlacement capability.ProviderPlacement) (Placement, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	switch providerPlacement.Scope {
	case capability.ScopeShared:
		project := ProviderProject
		volume := "baseharbor-prometheus-data"
		dir := filepath.Join(dataDir, "providers", "prometheus", "shared")
		if providerPlacement.SharingBoundary != "" {
			token := application.ProviderPlacementNameToken(providerPlacement.SharingBoundary)
			project += "-" + token
			volume += "-" + token
			dir = filepath.Join(dir, token)
		}
		return Placement{
			Scope:   capability.ScopeShared,
			Project: project,
			Volume:  volume,
			Dir:     dir,
		}, nil
	case capability.ScopeApplication:
		suffix := m.Name + "-" + m.Environment
		return Placement{
			Scope:   capability.ScopeApplication,
			Project: "baseharbor-metrics-" + suffix,
			Network: application.MetricsProviderNetworkName(m),
			Volume:  "baseharbor-prometheus-data-" + suffix,
			Dir:     filepath.Join(dataDir, "providers", "prometheus", "applications", m.Name, m.Environment),
		}, nil
	case capability.ScopeExternal:
		return Placement{Scope: capability.ScopeExternal}, nil
	default:
		return Placement{}, fmt.Errorf("unsupported Prometheus provider scope %q", providerPlacement.Scope)
	}
}

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DownProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type ProviderFiles struct {
	Dir           string
	Compose       string
	Env           string
	Config        string
	TargetsDir    string
	Registrations string
}

type sourceRegistration struct {
	Application   string `json:"application"`
	Environment   string `json:"environment"`
	Network       string `json:"network"`
	RuntimeVolume string `json:"runtime_volume,omitempty"`
}

type Driver struct {
	runtime Runtime
	app     application.Manifest
	client  *http.Client
}

func NewDriver(runtime Runtime, app application.Manifest) *Driver {
	return &Driver{
		runtime: runtime,
		app:     app,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Driver) Descriptor() capability.Provider { return capability.Prometheus }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Metrics {
		return fmt.Errorf("Prometheus provider cannot satisfy %s", resource.Kind)
	}
	if binding.Metrics == nil {
		return errors.New("metrics binding is required")
	}
	value := binding.Metrics
	if value.Direction != "provide" || value.Format != "openmetrics" {
		return errors.New("Prometheus provider requires provide/openmetrics metrics binding")
	}
	if value.Service == "" || value.Port < 1 || value.Port > 65535 || !strings.HasPrefix(value.Path, "/") {
		return errors.New("Prometheus metrics binding is incomplete")
	}
	policy, err := application.MetricsPolicy(d.app)
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return errors.New("metrics collection is disabled by deployment policy")
	}
	placement, err := application.ResolveProviderPlacement(d.app, capability.ProviderPrometheus)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return errors.New("external metrics provider requires an external collection adapter")
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	placement, err := PlacementFor(d.app)
	if err != nil {
		return err
	}
	files, err := EnsureProviderFiles(d.app)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate Prometheus provider configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start Prometheus provider: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	return waitReady(ctx, d.client, endpoint)
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if binding.Metrics == nil {
		return errors.New("metrics binding is required")
	}
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	target := targetGroup{
		Targets: []string{net.JoinHostPort(application.MetricsTargetAlias(d.app, binding.Metrics.Service), strconv.Itoa(binding.Metrics.Port))},
		Labels: map[string]string{
			"job":                     "baseharbor-applications",
			"baseharbor_application":  d.app.Name,
			"baseharbor_environment":  d.app.Environment,
			"baseharbor_service":      binding.Metrics.Service,
			"baseharbor_source":       resource.Name,
			"baseharbor_source_class": string(application.MetricsSourceApplication),
			"baseharbor_metrics_path": binding.Metrics.Path,
		},
	}
	data, err := json.MarshalIndent([]targetGroup{target}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(files.TargetsDir, targetFileName(d.app, resource.Name))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write Prometheus target: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace Prometheus target: %w", err)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		`up{job="baseharbor-applications",baseharbor_application=%q,baseharbor_environment=%q,baseharbor_source=%q}`,
		d.app.Name, d.app.Environment, resource.Name,
	)
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		ok, err := queryUp(deadline, d.client, endpoint, query)
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("Prometheus target has not produced an up=1 sample yet")
		}
		select {
		case <-deadline.Done():
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 2*time.Second)
			diagnostic := prometheusTargetDiagnostic(diagnosticCtx, d.client, endpoint, d.app, resource.Name)
			diagnosticCancel()
			if diagnostic != "" {
				return fmt.Errorf("verify Prometheus scrape for %s/%s: %s", d.app.Name, resource.Name, diagnostic)
			}
			if last == nil || errors.Is(last, context.DeadlineExceeded) {
				last = errors.New("Prometheus target has not produced an up=1 sample before verification deadline")
			}
			return fmt.Errorf("verify Prometheus scrape for %s/%s: %w", d.app.Name, resource.Name, last)
		case <-ticker.C:
		}
	}
}

func PruneApplicationTargets(m application.Manifest, desired map[string]struct{}) error {
	files, err := ExistingProviderFiles(m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return pruneApplicationTargetsFromFiles(files, m, desired)
}

func PruneRegisteredApplicationTargets(m application.Manifest, desired map[string]struct{}) error {
	files, found, err := ExistingRegisteredProviderFiles(m)
	if !found || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return pruneApplicationTargetsFromFiles(files, m, desired)
}

func pruneApplicationTargetsFromFiles(files ProviderFiles, m application.Manifest, desired map[string]struct{}) error {
	entries, err := os.ReadDir(files.TargetsDir)
	if err != nil {
		return err
	}
	prefix := targetFilePrefix(m)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if _, keep := desired[entry.Name()]; keep {
			continue
		}
		if err := os.Remove(filepath.Join(files.TargetsDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale Prometheus target %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func DesiredTargetFiles(m application.Manifest) map[string]struct{} {
	result := make(map[string]struct{}, len(m.Metrics.Sources))
	for _, source := range m.Metrics.Sources {
		result[targetFileName(m, source.Name)] = struct{}{}
	}
	return result
}

func providerTargetFileName(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("provider--%x.json", sum[:10])
}

func syncProviderTargets(dir string, sources []observability.MetricsSource) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "provider--") && strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	for _, source := range sources {
		target := targetGroup{
			Targets: []string{source.Target},
			Labels: map[string]string{
				"job":                     "baseharbor-providers",
				"baseharbor_provider":     string(source.Provider),
				"baseharbor_source":       source.ID,
				"baseharbor_source_class": string(source.Class),
				"baseharbor_metrics_path": source.Path,
			},
		}
		data, err := json.MarshalIndent([]targetGroup{target}, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(dir, providerTargetFileName(source.ID)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func providerMetricNetworks(sources []observability.MetricsSource) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, source := range sources {
		network := strings.TrimSpace(source.Network)
		if network == "" {
			continue
		}
		if _, exists := seen[network]; exists {
			continue
		}
		seen[network] = struct{}{}
		result = append(result, network)
	}
	sort.Strings(result)
	return result
}

func EnsureProviderFiles(m application.Manifest) (ProviderFiles, error) {
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external metrics provider has no BaseHarbor-owned provider files")
	}
	dir := placement.Dir
	targetsDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(targetsDir, 0o755); err != nil {
		return ProviderFiles{}, fmt.Errorf("create Prometheus provider state: %w", err)
	}
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), TargetsDir: targetsDir,
		Registrations: filepath.Join(dir, "registrations.json"),
	}
	registrations := []sourceRegistration{registrationFor(m)}
	if placement.Scope == capability.ScopeShared {
		registrations, err = reconcileSharedRegistration(files.Registrations, m, true)
		if err != nil {
			return ProviderFiles{}, err
		}
	}

	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	providerPlacement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return ProviderFiles{}, err
	}
	allowedApplications := []string{m.Name}
	if placement.Scope == capability.ScopeShared {
		allowedApplications = allowedApplications[:0]
		for _, registration := range registrations {
			allowedApplications = append(allowedApplications, registration.Application)
		}
	}
	providerSources, err := observability.ListMetrics(
		providerPlacement,
		allowedApplications,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil {
		return ProviderFiles{}, err
	}
	if err := syncProviderTargets(files.TargetsDir, providerSources); err != nil {
		return ProviderFiles{}, err
	}
	providerNetworks := providerMetricNetworks(providerSources)

	port := ""
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
				port = strings.TrimSpace(value)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	if port == "" {
		value, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		port = strconv.Itoa(value)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_PORT="+port+"\n"), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig(registrations)), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAMLWithProviderNetworks(placement, registrations, providerNetworks)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles(m application.Manifest) (ProviderFiles, error) {
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	return existingProviderFilesForPlacement(placement)
}

func ExistingRegisteredProviderFiles(m application.Manifest) (ProviderFiles, bool, error) {
	placement, found, err := RegisteredPlacementFor(m)
	if err != nil || !found {
		return ProviderFiles{}, found, err
	}
	files, err := existingProviderFilesForPlacement(placement)
	if err != nil {
		return ProviderFiles{}, true, err
	}
	return files, true, nil
}

func existingProviderFilesForPlacement(placement Placement) (ProviderFiles, error) {
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, os.ErrNotExist
	}
	return providerFilesAt(placement.Dir)
}

func UnregisterSharedApplication(ctx context.Context, runtime Runtime, m application.Manifest) error {
	placement, found, err := RegisteredPlacementFor(m)
	if err != nil {
		return err
	}
	if !found || placement.Scope != capability.ScopeShared {
		return nil
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	registrations, err := reconcileSharedRegistration(files.Registrations, m, false)
	if err != nil {
		return err
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig(registrations)), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAML(placement, registrations)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate shared Prometheus after application unregister: %w", err)
	}
	if err := runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("reconcile shared Prometheus after application unregister: %w", err)
	}
	return nil
}

func StopProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	placement, found, err := RegisteredPlacementFor(m)
	if err != nil {
		return err
	}
	if !found || placement.Scope != capability.ScopeApplication {
		return nil
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return runtime.DownProject(ctx, placement.Project, files.Compose, files.Env)
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	placement, found, err := RegisteredPlacementFor(m)
	if err != nil {
		return err
	}
	if !found || placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := existingProviderFilesForPlacement(placement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func SharedProbeManifest() application.Manifest {
	return application.Manifest{Name: "shared-probe", Environment: "dev"}
}

type SharedProviderInstance struct {
	Placement Placement
	Files     ProviderFiles
}

func ExistingSharedProviderInstances() ([]SharedProviderInstance, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return nil, err
	}
	root := filepath.Join(dataDir, "providers", "prometheus", "shared")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var instances []SharedProviderInstance
	if files, err := providerFilesAt(root); err == nil {
		instances = append(instances, SharedProviderInstance{
			Placement: Placement{
				Scope:   capability.ScopeShared,
				Project: ProviderProject,
				Volume:  "baseharbor-prometheus-data",
				Dir:     root,
			},
			Files: files,
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		token := entry.Name()
		dir := filepath.Join(root, token)
		files, err := providerFilesAt(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		instances = append(instances, SharedProviderInstance{
			Placement: Placement{
				Scope:   capability.ScopeShared,
				Project: ProviderProject + "-" + token,
				Volume:  "baseharbor-prometheus-data-" + token,
				Dir:     dir,
			},
			Files: files,
		})
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Placement.Project < instances[j].Placement.Project
	})
	return instances, nil
}

func providerFilesAt(dir string) (ProviderFiles, error) {
	files := ProviderFiles{
		Dir:           dir,
		Compose:       filepath.Join(dir, "compose.yaml"),
		Env:           filepath.Join(dir, "runtime.env"),
		Config:        filepath.Join(dir, "prometheus.yml"),
		TargetsDir:    filepath.Join(dir, "targets"),
		Registrations: filepath.Join(dir, "registrations.json"),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroyAllSharedProviders(ctx context.Context, runtime Runtime) error {
	instances, err := ExistingSharedProviderInstances()
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if err := runtime.DestroyProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
			return fmt.Errorf("destroy shared Prometheus project %s: %w", instance.Placement.Project, err)
		}
	}
	if len(instances) == 0 {
		return nil
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dataDir, "providers", "prometheus", "shared"))
}

func ExistingSharedProviderFiles() (ProviderFiles, error) {
	m := SharedProbeManifest()
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope != capability.ScopeShared {
		return ProviderFiles{}, os.ErrNotExist
	}
	dir := placement.Dir
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), TargetsDir: filepath.Join(dir, "targets"),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	m := SharedProbeManifest()
	return DestroyProvider(ctx, runtime, m)
}

func registrationFor(m application.Manifest) sourceRegistration {
	registration := sourceRegistration{
		Application: m.Name,
		Environment: m.Environment,
		Network:     application.MetricsProviderNetworkName(m),
	}
	if application.HasRuntimeMetricsPermissions(m) {
		registration.RuntimeVolume = application.MetricsRuntimeTargetVolumeName(m)
	}
	return registration
}

func readRegistrations(path string) ([]sourceRegistration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registrations []sourceRegistration
	if err := json.Unmarshal(data, &registrations); err != nil {
		return nil, errors.New("Prometheus shared registration state is invalid")
	}
	return registrations, nil
}

func reconcileSharedRegistration(path string, m application.Manifest, present bool) ([]sourceRegistration, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var registrations []sourceRegistration
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &registrations); err != nil {
			return nil, errors.New("Prometheus shared registration state is invalid")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	filtered := registrations[:0]
	for _, registration := range registrations {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			continue
		}
		filtered = append(filtered, registration)
	}
	registrations = filtered
	if present {
		registrations = append(registrations, registrationFor(m))
	}
	sort.Slice(registrations, func(i, j int) bool {
		if registrations[i].Application != registrations[j].Application {
			return registrations[i].Application < registrations[j].Application
		}
		return registrations[i].Environment < registrations[j].Environment
	})
	data, err := json.MarshalIndent(registrations, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return registrations, nil
}

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Prometheus port")
			}
			return "http://127.0.0.1:" + strconv.Itoa(port), nil
		}
	}
	return "", errors.New("Prometheus port is not materialized")
}

type targetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func targetFilePrefix(m application.Manifest) string {
	return m.Name + "--" + m.Environment + "--"
}

func targetFileName(m application.Manifest, source string) string {
	return targetFilePrefix(m) + source + ".json"
}

func providerComposeYAML(placement Placement, registrations []sourceRegistration) string {
	return providerComposeYAMLWithProviderNetworks(placement, registrations, nil)
}

func providerComposeYAMLWithProviderNetworks(placement Placement, registrations []sourceRegistration, providerNetworks []string) string {
	registrations = append([]sourceRegistration(nil), registrations...)
	sort.Slice(registrations, func(i, j int) bool {
		if registrations[i].Application != registrations[j].Application {
			return registrations[i].Application < registrations[j].Application
		}
		return registrations[i].Environment < registrations[j].Environment
	})

	var b strings.Builder
	b.WriteString("services:\n  prometheus:\n")
	fmt.Fprintf(&b, "    image: %s\n", ProviderImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65534:65534\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    command:\n")
	b.WriteString("      - --config.file=/etc/prometheus/prometheus.yml\n")
	b.WriteString("      - --storage.tsdb.path=/prometheus\n")
	b.WriteString("    ports:\n")
	b.WriteString("      - \"127.0.0.1:${BASEHARBOR_PROMETHEUS_PORT}:9090\"\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro\n")
	b.WriteString("      - ./targets:/etc/prometheus/targets:ro\n")
	b.WriteString("      - prometheus-data:/prometheus\n")
	for i, registration := range registrations {
		if registration.RuntimeVolume != "" {
			fmt.Fprintf(&b, "      - runtime-targets-%d:/etc/prometheus/runtime-targets/%d:ro\n", i, i)
		}
	}
	b.WriteString("    tmpfs:\n      - /tmp\n")
	b.WriteString("    cap_drop:\n      - ALL\n")
	b.WriteString("    security_opt:\n      - no-new-privileges:true\n")
	if len(registrations) > 0 || len(providerNetworks) > 0 {
		b.WriteString("    networks:\n")
		for i := range registrations {
			fmt.Fprintf(&b, "      - metrics-%d\n", i)
		}
		for i := range providerNetworks {
			fmt.Fprintf(&b, "      - provider-%d\n", i)
		}
		b.WriteString("\nnetworks:\n")
		for i, registration := range registrations {
			fmt.Fprintf(&b, "  metrics-%d:\n    name: %s\n", i, strconv.Quote(registration.Network))
		}
		for i, network := range providerNetworks {
			fmt.Fprintf(&b, "  provider-%d:\n    external: true\n    name: %s\n", i, strconv.Quote(network))
		}
	}
	b.WriteString("\nvolumes:\n")
	fmt.Fprintf(&b, "  prometheus-data:\n    name: %s\n", strconv.Quote(placement.Volume))
	for i, registration := range registrations {
		if registration.RuntimeVolume != "" {
			fmt.Fprintf(&b, "  runtime-targets-%d:\n    external: true\n    name: %s\n", i, strconv.Quote(registration.RuntimeVolume))
		}
	}
	return b.String()
}

func prometheusConfig(registrations []sourceRegistration) string {
	var b strings.Builder
	b.WriteString(`global:
  scrape_interval: 5s
  scrape_timeout: 4s

scrape_configs:
  - job_name: baseharbor-applications
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/*.json
`)
	for i, registration := range registrations {
		if registration.RuntimeVolume == "" {
			continue
		}
		fmt.Fprintf(&b, "          - /etc/prometheus/runtime-targets/%d/*.json\n", i)
	}
	b.WriteString(`        refresh_interval: 2s
    relabel_configs:
      - source_labels: [baseharbor_metrics_path]
        target_label: __metrics_path__
      - action: labeldrop
        regex: baseharbor_metrics_path
`)
	return b.String()
}

func waitReady(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/-/ready", nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func prometheusTargetDiagnostic(ctx context.Context, client *http.Client, endpoint string, app application.Manifest, source string) string {
	target := strings.TrimRight(endpoint, "/") + "/api/v1/targets?state=active"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ActiveTargets []struct {
				Labels    map[string]string `json:"labels"`
				Health    string            `json:"health"`
				LastError string            `json:"lastError"`
			} `json:"activeTargets"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.Status != "success" {
		return ""
	}
	for _, active := range payload.Data.ActiveTargets {
		if active.Labels["baseharbor_application"] != app.Name ||
			active.Labels["baseharbor_environment"] != app.Environment ||
			active.Labels["baseharbor_source"] != source {
			continue
		}
		if strings.TrimSpace(active.LastError) != "" {
			return fmt.Sprintf("target health=%s last_error=%s", active.Health, active.LastError)
		}
		return fmt.Sprintf("target health=%s but no up=1 sample was observed", active.Health)
	}
	return "Prometheus has no active target matching the application metrics binding"
}

func VerifyProviderSources(ctx context.Context, m application.Manifest) error {
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return err
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return err
	}
	allowedApplications := []string{m.Name}
	if placement.Scope == capability.ScopeShared {
		if files, fileErr := ExistingProviderFiles(m); fileErr == nil {
			if registrations, regErr := readRegistrations(files.Registrations); regErr == nil {
				allowedApplications = allowedApplications[:0]
				for _, registration := range registrations {
					allowedApplications = append(allowedApplications, registration.Application)
				}
			}
		}
	}
	sources, err := observability.ListMetrics(
		placement,
		allowedApplications,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil || len(sources) == 0 {
		return err
	}
	files, err := ExistingProviderFiles(m)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for _, source := range sources {
		query := fmt.Sprintf(
			`up{job="baseharbor-providers",baseharbor_provider=%q,baseharbor_source=%q}`,
			string(source.Provider), source.ID,
		)
		ticker := time.NewTicker(time.Second)
		var last error
		for {
			ok, err := queryUp(deadline, client, endpoint, query)
			if err == nil && ok {
				ticker.Stop()
				break
			}
			if err != nil {
				last = err
			} else {
				last = errors.New("provider target has not produced an up=1 sample yet")
			}
			select {
			case <-deadline.Done():
				ticker.Stop()
				return fmt.Errorf("verify provider metrics %s: %w", source.ID, last)
			case <-ticker.C:
			}
		}
	}
	return nil
}

func queryUp(ctx context.Context, client *http.Client, endpoint, query string) (bool, error) {
	values := url.Values{"query": []string{query}}
	target := strings.TrimRight(endpoint, "/") + "/api/v1/query?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return false, fmt.Errorf("Prometheus query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	if payload.Status != "success" {
		return false, errors.New("Prometheus query did not succeed")
	}
	for _, result := range payload.Data.Result {
		if len(result.Value) != 2 {
			continue
		}
		if value, ok := result.Value[1].(string); ok && value == "1" {
			return true, nil
		}
	}
	return false, nil
}

func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
