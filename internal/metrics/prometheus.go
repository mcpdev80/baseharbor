package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderProject = "baseharbor-metrics"
	ProviderService = "prometheus"
	ProviderImage   = "docker.io/prom/prometheus:v3.14.0"

	providerReconcileTimeout = 60 * time.Second
)

type Placement struct {
	Scope   capability.ProviderScope
	Project string
	Network string
	Volume  string
	Dir     string
}

func PlacementFor(m application.Manifest) (Placement, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	return PlacementForAt(dataDir, "", m)
}

func PlacementForAt(dataDir, namespace string, m application.Manifest) (Placement, error) {
	providerPlacement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return Placement{}, err
	}
	return placementFromProviderPlacementAt(dataDir, namespace, m, providerPlacement)
}

func RegisteredPlacementFor(m application.Manifest) (Placement, bool, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, false, err
	}
	return RegisteredPlacementForAt(dataDir, "", m)
}

func RegisteredPlacementForAt(dataDir, namespace string, m application.Manifest) (Placement, bool, error) {
	providerPlacement, found, err := application.RegisteredProviderPlacementAt(dataDir, m, capability.ProviderPrometheus)
	if err != nil || !found {
		return Placement{}, found, err
	}
	placement, err := placementFromProviderPlacementAt(dataDir, namespace, m, providerPlacement)
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
	return placementFromProviderPlacementAt(dataDir, "", m, providerPlacement)
}

func placementFromProviderPlacementAt(dataDir, namespace string, m application.Manifest, providerPlacement capability.ProviderPlacement) (Placement, error) {
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	prefix := ""
	if namespace != "" {
		prefix = namespace + "-"
	}
	switch providerPlacement.Scope {
	case capability.ScopeShared:
		project := ProviderProject
		volume := "baseharbor-prometheus-data"
		if prefix != "" {
			project = "baseharbor-metrics-" + strings.TrimSuffix(prefix, "-")
			volume = "baseharbor-prometheus-data-" + strings.TrimSuffix(prefix, "-")
		}
		dir := filepath.Join(filepath.Clean(dataDir), "providers", "prometheus", "shared")
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
		suffix := prefix + m.Name + "-" + m.Environment
		return Placement{
			Scope:   capability.ScopeApplication,
			Project: "baseharbor-metrics-" + suffix,
			Network: "baseharbor-metrics-" + suffix + "_default",
			Volume:  "baseharbor-prometheus-data-" + suffix,
			Dir:     filepath.Join(filepath.Clean(dataDir), "providers", "prometheus", "applications", m.Name, m.Environment),
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

type runtimeDiagnostics interface {
	DiagnosticsProject(context.Context, string, string, string) string
}

type ProviderFiles struct {
	Dir                 string
	Compose             string
	Env                 string
	Config              string
	TargetsDir          string
	ProviderSecurityDir string
	Registrations       string
	RuntimeCA           string
}

type sourceRegistration struct {
	Application   string `json:"application"`
	Environment   string `json:"environment"`
	Network       string `json:"network"`
	RuntimeVolume string `json:"runtime_volume,omitempty"`
}

type Driver struct {
	runtime   Runtime
	app       application.Manifest
	issuer    serviceaccess.Issuer
	runtimeCA string
	client    *http.Client
	dataDir   string
	namespace string
}

func NewDriver(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, runtimeCA ...string) *Driver {
	caPath := ""
	if len(runtimeCA) > 0 {
		caPath = strings.TrimSpace(runtimeCA[0])
	}
	return &Driver{
		runtime:   runtime,
		app:       app,
		issuer:    issuer,
		runtimeCA: caPath,
		client:    nil,
	}
}

func NewDriverAt(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string, runtimeCA ...string) *Driver {
	driver := NewDriver(runtime, app, issuer, runtimeCA...)
	driver.dataDir = filepath.Clean(dataDir)
	driver.namespace = strings.TrimSpace(namespace)
	return driver
}

func (d *Driver) placement() (Placement, error) {
	if d.dataDir != "" && d.dataDir != "." {
		return PlacementForAt(d.dataDir, d.namespace, d.app)
	}
	return PlacementFor(d.app)
}

func (d *Driver) ensureProviderFiles(ctx context.Context) (ProviderFiles, error) {
	if d.dataDir != "" && d.dataDir != "." {
		return EnsureProviderFilesWithRuntimeCAAt(ctx, d.issuer, d.dataDir, d.namespace, d.app, d.runtimeCA)
	}
	return EnsureProviderFilesWithRuntimeCA(ctx, d.issuer, d.app, d.runtimeCA)
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
	if value.Scheme != "http" && value.Scheme != "https" {
		return fmt.Errorf("Prometheus metrics binding has unsupported scheme %q", value.Scheme)
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
	reconcileCtx, cancel := context.WithTimeout(ctx, providerReconcileTimeout)
	defer cancel()

	placement, err := d.placement()
	if err != nil {
		return err
	}
	files, err := d.ensureProviderFiles(reconcileCtx)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(reconcileCtx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate Prometheus provider configuration: %w", err)
	}
	if err := d.runtime.UpProject(reconcileCtx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start Prometheus provider: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client, err := providerHTTPClient(d.app, files)
	if err != nil {
		return err
	}
	d.client = client
	if err := waitReady(reconcileCtx, d.client, endpoint); err != nil {
		if diagnostics, ok := d.runtime.(runtimeDiagnostics); ok {
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			detail := diagnostics.DiagnosticsProject(diagnosticCtx, placement.Project, files.Compose, files.Env)
			diagnosticCancel()
			if strings.TrimSpace(detail) != "" {
				return fmt.Errorf("wait for Prometheus readiness: %w\n%s", err, detail)
			}
		}
		return fmt.Errorf("wait for Prometheus readiness: %w", err)
	}
	if err := reloadConfig(reconcileCtx, d.client, endpoint); err != nil {
		return fmt.Errorf("reload Prometheus configuration: %w", err)
	}
	return nil
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if binding.Metrics == nil {
		return errors.New("metrics binding is required")
	}
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	labels := map[string]string{}
	labels["job"] = "baseharbor-applications"
	labels["baseharbor_application"] = d.app.Name
	labels["baseharbor_environment"] = d.app.Environment
	labels["baseharbor_service"] = binding.Metrics.Service
	labels["baseharbor_source"] = resource.Name
	labels["baseharbor_source_class"] = string(application.MetricsSourceApplication)
	labels["baseharbor_metrics_path"] = binding.Metrics.Path
	labels["baseharbor_metrics_scheme"] = binding.Metrics.Scheme
	target := targetGroup{
		Targets: []string{net.JoinHostPort(application.MetricsTargetAlias(d.app, binding.Metrics.Service), strconv.Itoa(binding.Metrics.Port))},
		Labels:  labels,
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
	if d.client == nil {
		d.client, err = providerHTTPClient(d.app, files)
		if err != nil {
			return err
		}
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
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return PruneApplicationTargetsAt(dataDir, "", m, desired)
}

func PruneApplicationTargetsAt(dataDir, namespace string, m application.Manifest, desired map[string]struct{}) error {
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return pruneApplicationTargetsFromFiles(files, m, desired)
}

func PruneRegisteredApplicationTargets(m application.Manifest, desired map[string]struct{}) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return PruneRegisteredApplicationTargetsAt(dataDir, "", m, desired)
}

func PruneRegisteredApplicationTargetsAt(dataDir, namespace string, m application.Manifest, desired map[string]struct{}) error {
	files, found, err := ExistingRegisteredProviderFilesAt(dataDir, namespace, m)
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
