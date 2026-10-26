package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type PrometheusInstance struct {
	Endpoint   string
	HTTPClient *http.Client
}

type PrometheusTarget struct {
	Application string
	Environment string
	Source      string
	Service     string
	Scheme      string
	Port        int
	Path        string
}

type PrometheusRealization interface {
	Apply(context.Context) (PrometheusInstance, error)
	Existing(context.Context) (PrometheusInstance, error)
	RegisterTarget(context.Context, PrometheusTarget) error
	Destroy(context.Context) error
}

type runtimePrometheusRealization struct {
	runtime   Runtime
	app       application.Manifest
	issuer    serviceaccess.Issuer
	runtimeCA string
	dataDir   string
	namespace string
}

func newRuntimePrometheusRealization(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, runtimeCA, dataDir, namespace string) PrometheusRealization {
	return &runtimePrometheusRealization{
		runtime: runtime, app: app, issuer: issuer, runtimeCA: strings.TrimSpace(runtimeCA),
		dataDir: strings.TrimSpace(dataDir), namespace: strings.TrimSpace(namespace),
	}
}

func (r *runtimePrometheusRealization) placement() (Placement, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return PlacementForAt(r.dataDir, r.namespace, r.app)
	}
	return PlacementFor(r.app)
}

func (r *runtimePrometheusRealization) ensureProviderFiles(ctx context.Context) (ProviderFiles, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return EnsureProviderFilesWithRuntimeCAAt(ctx, r.issuer, r.dataDir, r.namespace, r.app, r.runtimeCA)
	}
	return EnsureProviderFilesWithRuntimeCA(ctx, r.issuer, r.app, r.runtimeCA)
}

func (r *runtimePrometheusRealization) existingProviderFiles() (ProviderFiles, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return ExistingProviderFilesAt(r.dataDir, r.namespace, r.app)
	}
	return ExistingProviderFiles(r.app)
}

func (r *runtimePrometheusRealization) Apply(ctx context.Context) (PrometheusInstance, error) {
	reconcileCtx, cancel := context.WithTimeout(ctx, providerReconcileTimeout)
	defer cancel()

	placement, err := r.placement()
	if err != nil {
		return PrometheusInstance{}, err
	}
	files, err := r.ensureProviderFiles(reconcileCtx)
	if err != nil {
		return PrometheusInstance{}, err
	}
	if cleaner, ok := r.runtime.(legacyServiceCleaner); ok {
		if err := cleaner.RemoveProjectServices(reconcileCtx, placement.Project, "prometheus-access", "baseharbor-internal-prometheus-access"); err != nil {
			return PrometheusInstance{}, fmt.Errorf("remove legacy Prometheus access gateway: %w", err)
		}
	}
	if err := r.runtime.ConfigProject(reconcileCtx, placement.Project, files.Compose, files.Env); err != nil {
		return PrometheusInstance{}, fmt.Errorf("validate Prometheus provider configuration: %w", err)
	}
	if err := r.runtime.UpProject(reconcileCtx, placement.Project, files.Compose, files.Env); err != nil {
		return PrometheusInstance{}, fmt.Errorf("start Prometheus provider: %w", err)
	}
	instance, err := r.instance(files)
	if err != nil {
		return PrometheusInstance{}, err
	}
	if err := waitReady(reconcileCtx, instance.HTTPClient, instance.Endpoint); err != nil {
		if diagnostics, ok := r.runtime.(runtimeDiagnostics); ok {
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			detail := diagnostics.DiagnosticsProject(diagnosticCtx, placement.Project, files.Compose, files.Env)
			diagnosticCancel()
			if strings.TrimSpace(detail) != "" {
				return PrometheusInstance{}, fmt.Errorf("wait for Prometheus readiness: %w\n%s", err, detail)
			}
		}
		return PrometheusInstance{}, fmt.Errorf("wait for Prometheus readiness: %w", err)
	}
	if err := reloadConfig(reconcileCtx, instance.HTTPClient, instance.Endpoint); err != nil {
		return PrometheusInstance{}, fmt.Errorf("reload Prometheus configuration: %w", err)
	}
	return instance, nil
}

func (r *runtimePrometheusRealization) Existing(context.Context) (PrometheusInstance, error) {
	files, err := r.existingProviderFiles()
	if err != nil {
		return PrometheusInstance{}, err
	}
	return r.instance(files)
}

func (r *runtimePrometheusRealization) RegisterTarget(_ context.Context, target PrometheusTarget) error {
	if strings.TrimSpace(target.Application) == "" || strings.TrimSpace(target.Environment) == "" ||
		strings.TrimSpace(target.Source) == "" || strings.TrimSpace(target.Service) == "" {
		return errors.New("Prometheus target identity is incomplete")
	}
	if target.Port < 1 || target.Port > 65535 || !strings.HasPrefix(target.Path, "/") {
		return errors.New("Prometheus target endpoint is incomplete")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return fmt.Errorf("Prometheus target has unsupported scheme %q", target.Scheme)
	}
	files, err := r.existingProviderFiles()
	if err != nil {
		return err
	}
	labels := map[string]string{
		"job":                       "baseharbor-applications",
		"baseharbor_application":    target.Application,
		"baseharbor_environment":    target.Environment,
		"baseharbor_service":        target.Service,
		"baseharbor_source":         target.Source,
		"baseharbor_source_class":   string(application.MetricsSourceApplication),
		"baseharbor_metrics_path":   target.Path,
		"baseharbor_metrics_scheme": target.Scheme,
	}
	group := targetGroup{
		Targets: []string{net.JoinHostPort(application.MetricsTargetAliasFor(target.Application, target.Environment, target.Service), strconv.Itoa(target.Port))},
		Labels:  labels,
	}
	data, err := json.MarshalIndent([]targetGroup{group}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(files.TargetsDir, targetFileNameFor(target.Application, target.Environment, target.Source))
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

func (r *runtimePrometheusRealization) Destroy(ctx context.Context) error {
	placement, err := r.placement()
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := r.existingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.runtime.DestroyProject(ctx, placement.Project, files.Compose, files.Env)
}

func (r *runtimePrometheusRealization) instance(files ProviderFiles) (PrometheusInstance, error) {
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return PrometheusInstance{}, err
	}
	client, err := providerHTTPClient(r.app, files)
	if err != nil {
		return PrometheusInstance{}, err
	}
	return PrometheusInstance{Endpoint: endpoint, HTTPClient: client}, nil
}

func targetFileNameFor(applicationName, environment, source string) string {
	m := application.Manifest{Name: applicationName, Environment: environment}
	return targetFileName(m, source)
}
