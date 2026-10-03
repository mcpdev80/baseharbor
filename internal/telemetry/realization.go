package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type OTLPInstance struct {
	HostEndpoint      string
	WorkloadEndpoint  string
	TrustBundle       []byte
	ClientCertificate []byte
	ClientKey         []byte
	HTTPClient        *http.Client
	Network           string
	ObservationID     string
}

type OTLPRealization interface {
	Apply(context.Context) (OTLPInstance, error)
	Existing(context.Context) (OTLPInstance, error)
	Destroy(context.Context) error
}

type runtimeOTLPRealization struct {
	runtime       Runtime
	app           application.Manifest
	issuer        serviceaccess.Issuer
	traceEndpoint string
	traceNetwork  string
	dataDir       string
	namespace     string
}

func newRuntimeOTLPRealization(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) OTLPRealization {
	return &runtimeOTLPRealization{
		runtime: runtime, app: app, issuer: issuer,
		dataDir: strings.TrimSpace(dataDir), namespace: strings.TrimSpace(namespace),
	}
}

func (r *runtimeOTLPRealization) SetTraceBackend(endpoint, network string) {
	r.traceEndpoint = strings.TrimSpace(endpoint)
	r.traceNetwork = strings.TrimSpace(network)
}

func (r *runtimeOTLPRealization) providerFiles(ctx context.Context, ensure bool) (ProviderFiles, error) {
	if ensure {
		if r.dataDir != "" && r.dataDir != "." {
			return EnsureProviderFilesWithTraceBackendForEnvironmentAt(ctx, r.issuer, r.traceEndpoint, r.traceNetwork, r.app.Environment, r.dataDir, r.namespace)
		}
		return EnsureProviderFilesWithTraceBackendForEnvironment(ctx, r.issuer, r.traceEndpoint, r.traceNetwork, r.app.Environment)
	}
	if r.dataDir != "" && r.dataDir != "." {
		return ExistingProviderFilesAt(r.dataDir, r.namespace)
	}
	return ExistingProviderFiles()
}

func (r *runtimeOTLPRealization) Apply(ctx context.Context) (OTLPInstance, error) {
	files, err := r.providerFiles(ctx, true)
	if err != nil {
		return OTLPInstance{}, err
	}
	if cleaner, ok := r.runtime.(legacyServiceCleaner); ok {
		if err := cleaner.RemoveProjectServices(ctx, files.Project, "otel-collector-access"); err != nil {
			return OTLPInstance{}, fmt.Errorf("remove legacy OpenTelemetry access gateway: %w", err)
		}
	}
	if err := r.runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return OTLPInstance{}, fmt.Errorf("validate OpenTelemetry Collector configuration: %w", err)
	}
	if err := r.runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return OTLPInstance{}, fmt.Errorf("start OpenTelemetry Collector: %w", err)
	}
	instance, err := r.instance(files)
	if err != nil {
		return OTLPInstance{}, err
	}
	if err := waitOTLP(ctx, instance.HTTPClient, instance.HostEndpoint); err != nil {
		return OTLPInstance{}, err
	}
	return instance, nil
}

func (r *runtimeOTLPRealization) Existing(ctx context.Context) (OTLPInstance, error) {
	files, err := r.providerFiles(ctx, false)
	if err != nil {
		return OTLPInstance{}, err
	}
	return r.instance(files)
}

func (r *runtimeOTLPRealization) Destroy(ctx context.Context) error {
	var dataDir string
	if r.dataDir != "" && r.dataDir != "." {
		dataDir = filepath.Clean(r.dataDir)
	}
	if dataDir == "" {
		return DestroySharedProvider(ctx, r.runtime)
	}
	return DestroySharedProviderAt(ctx, r.runtime, dataDir, r.namespace)
}

func (r *runtimeOTLPRealization) instance(files ProviderFiles) (OTLPInstance, error) {
	hostEndpoint, err := providerEndpoint(files)
	if err != nil {
		return OTLPInstance{}, err
	}
	client, err := managedOTLPHTTPClient(r.app.Environment, files)
	if err != nil {
		return OTLPInstance{}, err
	}
	policy, err := serviceaccess.Resolve(r.app.Environment, "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return OTLPInstance{}, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return OTLPInstance{}, fmt.Errorf("load managed OTLP TLS material: %w", err)
	}
	read := func(path string) ([]byte, error) {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, nil
		}
		return os.ReadFile(path)
	}
	ca, err := read(material.CA)
	if err != nil {
		return OTLPInstance{}, err
	}
	cert, err := read(material.ClientCertificate)
	if err != nil {
		return OTLPInstance{}, err
	}
	key, err := read(material.ClientKey)
	if err != nil {
		return OTLPInstance{}, err
	}
	containerHost := "otel-collector"
	if policy.PKISource != serviceaccess.PKIManagedLocal && strings.TrimSpace(policy.ServerName) != "" {
		containerHost = strings.TrimSpace(policy.ServerName)
	}
	return OTLPInstance{
		HostEndpoint:      hostEndpoint,
		WorkloadEndpoint:  "https://" + containerHost + ":4318",
		TrustBundle:       ca,
		ClientCertificate: cert,
		ClientKey:         key,
		HTTPClient:        client,
		Network:           files.Network,
		ObservationID:     "opentelemetry-collector:" + files.Project,
	}, nil
}

func registerOTLPObservation(app application.Manifest, instance OTLPInstance) error {
	metricsPolicy, err := application.MetricsPolicy(app)
	if err != nil {
		return err
	}
	metricsEnabled := (application.HasMetricsSources(app) || application.HasRuntimeMetricsPermissions(app)) &&
		metricsPolicy.Enabled && metricsPolicy.Collect[application.MetricsSourcePlatformProvider]
	signals := map[string]observability.ProviderSignalRuntime{}
	if metricsEnabled {
		signals["collector-metrics"] = observability.ProviderSignalRuntime{
			Network: instance.Network,
			Target:  ProviderService + ":8888",
		}
	}
	return observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
		ID:         instance.ObservationID,
		Descriptor: capability.OTelCollectorIntegration,
		Class:      observability.SourcePlatformProvider,
		Scope:      capability.ScopeShared,
		Enabled:    map[observability.SignalKind]bool{observability.SignalMetrics: metricsEnabled},
		Signals:    signals,
	})
}

func (d *Driver) RotatePKI(ctx context.Context) error {
	if d.runtime == nil || d.issuer == nil {
		return errors.New("managed OTLP PKI rotation requires runtime and issuer")
	}
	reconcile := func() (ProviderFiles, error) {
		files, err := d.ensureProviderFiles(ctx)
		if err != nil {
			return ProviderFiles{}, err
		}
		if err := d.runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return ProviderFiles{}, err
		}
		if err := d.runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return ProviderFiles{}, err
		}
		endpoint, err := providerEndpoint(files)
		if err != nil {
			return ProviderFiles{}, err
		}
		client, err := managedOTLPHTTPClient(d.app.Environment, files)
		if err != nil {
			return ProviderFiles{}, err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := waitOTLP(verifyCtx, client, endpoint); err != nil {
			return ProviderFiles{}, fmt.Errorf("verify OTLP after PKI reconcile: %w", err)
		}
		d.client = client
		return files, nil
	}

	files, err := reconcile()
	if err != nil {
		return fmt.Errorf("reconcile replacement OTLP PKI with overlap: %w", err)
	}
	accessPolicy, err := serviceaccess.Resolve(d.app.Environment, "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	accessPolicy.ServerName = "otel-collector"
	memberPolicy := accessPolicy
	memberPolicy.AuthenticationRequired = true
	memberPolicy.Authentication = serviceaccess.AuthenticationMTLS
	for _, item := range []struct {
		name   string
		policy serviceaccess.Policy
		dir    string
	}{
		{name: "frontend", policy: accessPolicy, dir: filepath.Join(files.Dir, "service-access", "pki")},
		{name: "members", policy: memberPolicy, dir: filepath.Join(files.Dir, "members", "service-access", "pki")},
	} {
		if err := serviceaccess.RetireTLSOverlap(ctx, d.issuer, item.policy, item.dir); err != nil {
			return fmt.Errorf("retire previous OTLP %s CA: %w", item.name, err)
		}
	}
	if _, err := reconcile(); err != nil {
		return fmt.Errorf("reconcile OTLP after CA retirement: %w", err)
	}
	return nil
}
