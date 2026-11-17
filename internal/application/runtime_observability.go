package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func ManagedRuntimeProviderServiceNames(m Manifest) []string {
	services := make([]string, 0, len(SQLInstanceNames(m))+len(ValkeyInstanceNames(m)))
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			services = append(services, runtimeServiceName("postgres", instance))
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			services = append(services, runtimeServiceName("valkey", instance))
		}
	}
	return services
}

func reconcileManagedRuntimeObservability(m Manifest, project string) error {
	logsPolicy, err := LogsPolicy(m)
	if err != nil {
		return err
	}
	logsEnabled := logsPolicy.Enabled && logsPolicy.Collect[LogsSourceApplicationProvider]
	tracesPolicy, err := TracesPolicy(m)
	if err != nil {
		return err
	}
	tracesEnabled := tracesPolicy.Enabled && tracesPolicy.Collect[TracesSourceApplicationProvider]
	project = strings.TrimSpace(project)
	if project == "" {
		project = RuntimeProjectName(m)
	}

	postgresInstances := SQLInstanceNames(m)
	if UsesSharedPostgreSQL(m) {
		postgresInstances = nil
	}
	valkeyInstances := ValkeyInstanceNames(m)
	if UsesSharedValkey(m) {
		valkeyInstances = nil
	}
	if err := reconcileRuntimeProviderObservability(m, project, "postgres", capability.ProviderPostgreSQL, capability.PostgreSQLIntegration, postgresInstances, logsEnabled, tracesEnabled); err != nil {
		return err
	}
	if err := reconcileRuntimeProviderObservability(m, project, "valkey", capability.ProviderValkey, capability.ValkeyIntegration, valkeyInstances, logsEnabled, tracesEnabled); err != nil {
		return err
	}
	return reconcileApplicationMetricsSource(m, project, logsEnabled, tracesEnabled)
}

// reconcileApplicationMetricsSource registers the application's own runtime
// metrics source so that even a workload-only app (no SQL/Valkey backend, no
// runtime broker, no runtime metrics permission) declares an application-scoped
// metrics source in the shared provider registry. This keeps the shared
// runtime-sources.json snapshot non-empty for the owning app and lets metrics
// verification resolve the app's presence through the real deploy path instead
// of an empty snapshot.
//
// The source is registered only when the metrics policy enables collection and
// collects application-provider sources; a disabled policy stays a no-op and
// never fabricates a scrape target. It uses the same direct SignalSource
// construction as ReconcileRuntimeComponentObservability so it satisfies the
// metrics-source contract (openmetrics, absolute path, network) without a
// backend to scrape.
func reconcileApplicationMetricsSource(
	m Manifest,
	project string,
	logsEnabled bool,
	tracesEnabled bool,
) error {
	metricsPolicy, err := MetricsPolicy(m)
	if err != nil {
		return err
	}
	if !metricsPolicy.Enabled || !metricsPolicy.Collect[MetricsSourceApplicationProvider] {
		return nil
	}
	id := fmt.Sprintf("%s:%s", capability.ProviderPrometheus, strings.TrimSpace(project))
	desired := []observability.SignalSource{
		{
			ID:               id,
			Kind:             observability.SignalMetrics,
			Provider:         capability.ProviderPrometheus,
			Class:            observability.SourceApplicationProvider,
			Scope:            capability.ScopeApplication,
			OwnerApplication: m.Name,
			Network:          MetricsProviderNetworkNameForNamespace(m, ""),
			Target:           observability.RuntimeTarget(project, "workload"),
			Protocol:         "openmetrics",
			Path:             "/metrics",
			Mode:             capability.ObservabilityNative,
			Verification:     capability.ObservabilityVerifyBackend,
		},
	}
	if logsEnabled {
		desired = append(desired, observability.SignalSource{
			ID:                 id,
			Kind:               observability.SignalLogs,
			Provider:           capability.ProviderPrometheus,
			Class:              observability.SourceApplicationProvider,
			Scope:              capability.ScopeApplication,
			OwnerApplication:   m.Name,
			Target:             observability.RuntimeTarget(project, "workload"),
			Protocol:           "stdout-stderr",
			Mode:               capability.ObservabilityRuntime,
			SemanticConvention: "baseharbor.runtime.logs",
			Verification:       capability.ObservabilityVerifyBackend,
		})
	}
	if tracesEnabled {
		desired = append(desired, observability.SignalSource{
			ID:                 id,
			Kind:               observability.SignalTraces,
			Provider:           capability.ProviderPrometheus,
			Class:              observability.SourceApplicationProvider,
			Scope:              capability.ScopeApplication,
			OwnerApplication:   m.Name,
			Target:             observability.RuntimeTarget(project, "workload"),
			Protocol:           "otlp",
			Mode:               capability.ObservabilityNative,
			SemanticConvention: "http",
			Verification:       capability.ObservabilityVerifySpan,
		})
	}
	if len(desired) == 0 {
		return nil
	}
	return observability.ReconcileSignals(id, desired)
}

func reconcileRuntimeProviderObservability(
	m Manifest,
	project string,
	serviceKind string,
	provider capability.ProviderKind,
	descriptor capability.IntegrationDescriptor,
	instances []string,
	logsEnabled bool,
	tracesEnabled bool,
) error {
	keep := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		service := runtimeServiceName(serviceKind, instance)
		id := fmt.Sprintf("%s:%s:%s", provider, project, service)
		keep[id] = struct{}{}

		enabled := map[observability.SignalKind]bool{
			observability.SignalLogs:   logsEnabled,
			observability.SignalTraces: tracesEnabled,
		}
		signals := map[string]observability.ProviderSignalRuntime{}
		target := observability.RuntimeTarget(project, service)
		if logsEnabled {
			signals["logs"] = observability.ProviderSignalRuntime{Target: target}
		}
		if tracesEnabled {
			signals["traces"] = observability.ProviderSignalRuntime{Target: target}
		}
		if err := observability.RegisterProviderSignals(observability.ProviderSignalRegistration{
			ID:               id,
			Descriptor:       descriptor,
			Class:            observability.SourceApplicationProvider,
			Scope:            capability.ScopeApplication,
			OwnerApplication: m.Name,
			Enabled:          enabled,
			Signals:          signals,
		}); err != nil {
			return err
		}
	}
	return observability.PruneOwnedProviderInstances(provider, observability.SourceApplicationProvider, capability.ScopeApplication, m.Name, keep)
}

func VerifyManagedProviderInteractions(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, sources []observability.SignalSource) error {
	needsPostgreSQL := false
	needsValkey := false
	project := strings.TrimSpace(files.Project)
	if project == "" {
		project = RuntimeProjectName(m)
	}
	for _, source := range sources {
		if source.Kind != observability.SignalTraces || source.Mode != capability.ObservabilityInteraction {
			continue
		}
		sourceProject, _, ok := observability.ParseRuntimeTarget(source.Target)
		if !ok || sourceProject != project {
			return fmt.Errorf("provider interaction source %q has invalid runtime target %q", source.ID, source.Target)
		}
		switch source.Provider {
		case capability.ProviderPostgreSQL:
			needsPostgreSQL = true
		case capability.ProviderValkey:
			needsValkey = true
		default:
			return fmt.Errorf("provider interaction source %q has no runtime verification adapter", source.ID)
		}
	}
	if needsPostgreSQL {
		if err := VerifyPostgresRuntime(ctx, runtime, m, files); err != nil {
			return fmt.Errorf("verify PostgreSQL provider interaction: %w", err)
		}
	}
	if needsValkey {
		if err := VerifyValkeyRuntime(ctx, runtime, m, files); err != nil {
			return fmt.Errorf("verify Valkey provider interaction: %w", err)
		}
	}
	return nil
}
