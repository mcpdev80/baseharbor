package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type LokiInstance struct {
	Endpoint   string
	HTTPClient *http.Client
}

type LogSource struct {
	Application string
	Environment string
	Class       application.LogsSourceClass
	Provider    capability.ProviderKind
	Service     string
}

type lokiProjectDiagnostics interface {
	DiagnosticsProject(context.Context, string, string, string) string
}

type LokiRealization interface {
	Apply(context.Context) (LokiInstance, error)
	Existing(context.Context) (LokiInstance, error)
	RegisterSource(context.Context, LogSource) error
	VerifySource(context.Context, LogSource) error
	Destroy(context.Context) error
}

type runtimeLokiRealization struct {
	runtime   Runtime
	mode      bhruntime.LogCollectionMode
	app       application.Manifest
	issuer    serviceaccess.Issuer
	dataDir   string
	namespace string
}

func newRuntimeLokiRealization(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) LokiRealization {
	return &runtimeLokiRealization{
		runtime:   runtime,
		mode:      logCollectionMode(runtime),
		app:       app,
		issuer:    issuer,
		dataDir:   strings.TrimSpace(dataDir),
		namespace: strings.TrimSpace(namespace),
	}
}

func (r *runtimeLokiRealization) placement() (Placement, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return PlacementForAt(r.dataDir, r.namespace, r.app)
	}
	return PlacementFor(r.app)
}

func (r *runtimeLokiRealization) ensureProviderFiles(ctx context.Context) (ProviderFiles, error) {
	dataDir := r.dataDir
	if dataDir == "" || dataDir == "." {
		var err error
		dataDir, err = bhruntime.DataDir("")
		if err != nil {
			return ProviderFiles{}, err
		}
	}
	if !r.app.HA {
		return EnsureProviderFilesForModeAt(ctx, r.issuer, dataDir, r.namespace, r.app, r.mode)
	}
	storageRuntime, ok := r.runtime.(objectstorage.Runtime)
	if !ok {
		return ProviderFiles{}, errors.New("Loki HA requires runtime object-storage administration support")
	}
	bucket, err := objectstorage.EnsurePlatformBucketAt(ctx, storageRuntime, r.issuer, dataDir, r.namespace, "loki")
	if err != nil {
		return ProviderFiles{}, fmt.Errorf("prepare Loki HA object storage: %w", err)
	}
	return ensureProviderFilesForModeAt(ctx, r.issuer, dataDir, r.namespace, r.app, r.mode, &bucket)
}

func (r *runtimeLokiRealization) existingProviderFiles() (ProviderFiles, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return ExistingProviderFilesAt(r.dataDir, r.namespace, r.app)
	}
	return ExistingProviderFiles(r.app)
}

func (r *runtimeLokiRealization) applicationRegistration() (Registration, error) {
	if r.dataDir != "" && r.dataDir != "." {
		return ApplicationRegistrationAt(r.dataDir, r.namespace, r.app)
	}
	return ApplicationRegistration(r.app)
}

func (r *runtimeLokiRealization) Apply(ctx context.Context) (LokiInstance, error) {
	files, err := r.ensureProviderFiles(ctx)
	if err != nil {
		return LokiInstance{}, err
	}
	placement, err := r.placement()
	if err != nil {
		return LokiInstance{}, err
	}
	if err := r.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return LokiInstance{}, fmt.Errorf("validate Loki provider configuration: %w", err)
	}
	if err := r.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return LokiInstance{}, fmt.Errorf("start Loki provider: %w", err)
	}
	instance, err := r.instance(files)
	if err != nil {
		return LokiInstance{}, err
	}
	if err := waitLokiReady(ctx, instance.HTTPClient, instance.Endpoint); err != nil {
		if diagnostics, ok := r.runtime.(lokiProjectDiagnostics); ok {
			diagnosticCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			detail := strings.TrimSpace(diagnostics.DiagnosticsProject(diagnosticCtx, placement.Project, files.Compose, files.Env))
			cancel()
			if detail != "" {
				return LokiInstance{}, fmt.Errorf("Loki readiness: %w\n%s", err, detail)
			}
		}
		return LokiInstance{}, fmt.Errorf("Loki readiness: %w", err)
	}
	if err := r.registerProviderSignals(placement); err != nil {
		return LokiInstance{}, err
	}
	return instance, nil
}

func (r *runtimeLokiRealization) Existing(context.Context) (LokiInstance, error) {
	files, err := r.existingProviderFiles()
	if err != nil {
		return LokiInstance{}, err
	}
	return r.instance(files)
}

func (r *runtimeLokiRealization) RegisterSource(_ context.Context, source LogSource) error {
	if strings.TrimSpace(source.Application) == "" || strings.TrimSpace(source.Environment) == "" ||
		strings.TrimSpace(source.Service) == "" || source.Class == "" {
		return errors.New("log source identity is incomplete")
	}
	if source.Application != r.app.Name || source.Environment != r.app.Environment {
		return errors.New("log source does not belong to realization application")
	}
	if source.Class != application.LogsSourceApplication {
		return fmt.Errorf("runtime Loki application registration does not support source class %q", source.Class)
	}
	_, err := r.applicationRegistration()
	return err
}

func (r *runtimeLokiRealization) VerifySource(ctx context.Context, source LogSource) error {
	if source.Application != r.app.Name || source.Environment != r.app.Environment || strings.TrimSpace(source.Service) == "" {
		return errors.New("log source identity does not match realization application")
	}
	instance, err := r.Existing(ctx)
	if err != nil {
		return err
	}
	return waitForStream(ctx, instance.HTTPClient, instance.Endpoint, r.app, source.Service)
}

func (r *runtimeLokiRealization) Destroy(ctx context.Context) error {
	if r.dataDir != "" && r.dataDir != "." {
		return DestroyProviderAt(ctx, r.runtime, filepath.Clean(r.dataDir), r.namespace, r.app)
	}
	return DestroyProvider(ctx, r.runtime, r.app)
}

func (r *runtimeLokiRealization) instance(files ProviderFiles) (LokiInstance, error) {
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return LokiInstance{}, err
	}
	client, err := lokiHTTPClient(r.app, files)
	if err != nil {
		return LokiInstance{}, err
	}
	return LokiInstance{Endpoint: endpoint, HTTPClient: client}, nil
}

func (r *runtimeLokiRealization) registerProviderSignals(placement Placement) error {
	class := observability.SourcePlatformProvider
	if placement.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	metricsPolicy, err := application.MetricsPolicy(r.app)
	if err != nil {
		return err
	}
	metricsEnabled := (application.HasMetricsSources(r.app) || application.HasRuntimeMetricsPermissions(r.app)) && metricsPolicy.Enabled
	if class == observability.SourceApplicationProvider {
		metricsEnabled = metricsEnabled && metricsPolicy.Collect[application.MetricsSourceApplicationProvider]
	} else {
		metricsEnabled = metricsEnabled && metricsPolicy.Collect[application.MetricsSourcePlatformProvider]
	}
	signals := map[string]observability.ProviderSignalRuntime{}
	if metricsEnabled {
		signals["loki-metrics"] = observability.ProviderSignalRuntime{
			Network: placement.Network,
			Target:  "loki:3100",
		}
	}
	return observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
		ID:               "loki:" + placement.Project,
		Descriptor:       capability.LokiIntegration,
		Class:            class,
		Scope:            placement.Scope,
		SharingBoundary:  placement.SharingBoundary,
		OwnerApplication: placement.OwnerApplication,
		Enabled:          map[observability.SignalKind]bool{observability.SignalMetrics: metricsEnabled},
		Signals:          signals,
	})
}

func lokiHTTPClient(m application.Manifest, files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(m.Environment, "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load Loki service access identity: %w", err)
	}
	return serviceaccess.NewHTTPClientForPolicy(material, policy)
}
