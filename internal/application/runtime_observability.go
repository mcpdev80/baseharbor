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
	if err := reconcileApplicationMetricsSource(m, project); err != nil {
		return err
	}
	// The application-class log writer is its own boundary inside this
	// package: the workload's stream is registered directly against the
	// signal registry (internal/logs documents the identity contract it
	// shares with the driver's Bind path). It runs before any metrics or
	// traces policy decision, and a disabled logs policy reclaims the signal
	// instead of leaving a stale source behind.
	if err := reconcileApplicationLogSource(m, project); err != nil {
		return err
	}
	return reconcileApplicationTraceSource(m, project, tracesEnabled)
}

// ReconcileManagedRuntimeObservability is the exported form of the render-time
// reconciliation boundary (runtime_postgres.go calls the unexported one on
// every managed Compose render). The cross-package writer regression of card
// 0428-122 drives exactly this entry: the application-class log signal must
// survive repeated renders.
func ReconcileManagedRuntimeObservability(m Manifest, project string) error {
	return reconcileManagedRuntimeObservability(m, project)
}

// reconcileApplicationLogSource writes the class-application log signal of
// this deployment's workload identity into the shared observability registry.
// The collector has always labelled the workload's stream
// baseharbor_source_class="application" (internal/logs/render.go, both the
// syslog listener and the journald source), and #1680's reclaim proved no
// provider class may own that stream - but no writer ever stamped the class
// on a log signal, so the documented normalized class existed only in the
// contract and every application-class listing stayed silently empty
// (card 0428-122). This is the render-time writer for BOTH transports.
func reconcileApplicationLogSource(m Manifest, project string) error {
	policy, err := LogsPolicy(m)
	if err != nil {
		return err
	}
	desired := []observability.SignalSource{}
	if policy.Enabled && policy.Collect[LogsSourceApplication] {
		source := observability.SignalSource{
			ID:                 observability.ApplicationLogSourceID(m.Name, m.Environment, "workload"),
			Kind:               observability.SignalLogs,
			Provider:           capability.ProviderLoki,
			Class:              observability.SourceApplication,
			Scope:              capability.ScopeApplication,
			OwnerApplication:   m.Name,
			Target:             observability.RuntimeTarget(project, "workload"),
			Protocol:           "stdout-stderr",
			Mode:               capability.ObservabilityRuntime,
			SemanticConvention: "baseharbor.runtime.logs",
			Verification:       capability.ObservabilityVerifyBackend,
		}
		if err := source.Validate(); err != nil {
			return fmt.Errorf("application log source: %w", err)
		}
		desired = append(desired, source)
	}
	return observability.ReconcileMatching(observability.ApplicationLogBoundary(m.Name, m.Environment), desired)
}

// reconcileApplicationMetricsSource records the workload's runtime identity in
// the shared provider snapshot even when the app has no declared metrics
// endpoint. Its runtime target is symbolic, so it must not become a Prometheus
// file_sd scrape target or require a provider network connection. Application
// workload logs are collected through their application source; Prometheus is
// not a runtime log producer, so it must not claim a provider log source for
// the workload identity.
//
// A trace signal never belongs to this Prometheus-owned identity: Prometheus
// ingests metrics, not spans. The workload's native trace signal is registered
// under the OTLP receiver its telemetry policy actually selects, in its own
// reconciliation below, so span verification reaches an adapter that can export
// to a real receiver instead of failing on a metrics backend.
func reconcileApplicationMetricsSource(m Manifest, project string) error {
	// The legacy Prometheus-attributed trace signal is reclaimed before any
	// metrics-policy decision, and the equally invalid Prometheus-attributed
	// workload log source is reclaimed before any policy decision. Logs emitted
	// by the workload are collected under the application source; the Prometheus
	// workload identity has no separate runtime service that can emit provider
	// logs. Metrics are preserved when their collection policy is disabled.
	if err := pruneWorkloadTraceSignals(project, []capability.ProviderKind{capability.ProviderPrometheus}); err != nil {
		return err
	}
	id := fmt.Sprintf("%s:%s", capability.ProviderPrometheus, strings.TrimSpace(project))
	if err := observability.PruneProviderSignals(id, capability.ProviderPrometheus, observability.SignalLogs); err != nil {
		return err
	}
	metricsPolicy, err := MetricsPolicy(m)
	if err != nil {
		return err
	}
	if !metricsPolicy.Enabled || !metricsPolicy.Collect[MetricsSourceApplicationProvider] {
		return nil
	}
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
			Mode:             capability.ObservabilityRuntime,
			Verification:     capability.ObservabilityVerifyNone,
		},
	}

	if len(desired) == 0 {
		return nil
	}
	return observability.ReconcileSignals(id, desired)
}

// reconcileApplicationTraceSource registers the workload's native trace signal
// under the OTLP receiver that actually receives its spans. Prometheus is a
// metrics backend and Tempo stores traces without receiving them, so naming
// either of them here would leave span verification with a source no probe
// adapter can serve. The receiver identity comes from the same deployment
// policy the OTLP transport selection uses, so registry and exporter agree
// before, during and after a managed/external receiver transition.
func reconcileApplicationTraceSource(m Manifest, project string, tracesEnabled bool) error {
	project = strings.TrimSpace(project)
	if !HasOTLPTelemetry(m) {
		// The authored intent no longer requests OTLP delivery. Reclaim this
		// workload's receiver-owned trace signals before returning: the earlier
		// managed deployment registered them, and no other cleanup path sees
		// this identity. Metrics and log signals stay registered because their
		// providers still ingest them.
		return pruneWorkloadTraceSignals(project, workloadReceiverKinds)
	}
	if !tracesEnabled {
		if TelemetryProviderForDeployment().Kind == capability.ProviderExternalOTLP {
			// An external receiver delivers its spans without managed trace
			// storage (see requiresManagedTraceStorage), so this reconciliation
			// never registered a workload source under that receiver. Reclaiming
			// its identity here would delete a signal the managed collector
			// transport still provides. With the managed collector as the
			// receiver, the disabled policy does reclaim this workload's own
			// collector identity.
			return nil
		}
		// The deployment stopped collecting provider traces. Reclaim the trace
		// signal of this workload's own receiver identities so verification can
		// never reach a stale registration; the metrics and log signals of those
		// same identities stay registered.
		return pruneWorkloadTraceSignals(project, workloadReceiverKinds)
	}
	receiver, err := workloadTraceReceiver(m)
	if err != nil {
		return err
	}
	// The consumer identity follows the receiver: an external replacement must
	// not keep verifying through the managed collector, and returning to the
	// managed transport must not keep verifying a retired external source.
	// Only this workload's own receiver-owned identities are reclaimed, so a
	// shared collector or external source registration of another application
	// is never touched.
	if err := pruneWorkloadTraceSignals(project, otherReceiverKinds(receiver)); err != nil {
		return err
	}
	id := fmt.Sprintf("%s:%s", receiver, project)
	desired := []observability.SignalSource{{
		ID:                 id,
		Kind:               observability.SignalTraces,
		Provider:           receiver,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, "workload"),
		Protocol:           "otlp",
		Mode:               capability.ObservabilityNative,
		SemanticConvention: "http",
		Verification:       capability.ObservabilityVerifySpan,
	}}
	return observability.ReconcileSignals(id, desired)
}

// workloadTraceReceiver names the OTLP receiver the deployment policy selects
// for this application's spans. The selection is the one every other component
// reads, never a guess from leftover files, so the registry and the transport
// cannot disagree across an apply, a replacement or a return.
func workloadTraceReceiver(m Manifest) (capability.ProviderKind, error) {
	return TelemetryProviderForDeployment().Kind, nil
}

// workloadReceiverKinds are the OTLP receiver identities a workload's native
// trace signal can be registered under.
var workloadReceiverKinds = []capability.ProviderKind{capability.ProviderOTelCollector, capability.ProviderExternalOTLP}

// otherReceiverKinds returns the receiver identities a transition away from the
// given receiver must reclaim, excluding the receiver that stays registered.
func otherReceiverKinds(receiver capability.ProviderKind) []capability.ProviderKind {
	var others []capability.ProviderKind
	for _, kind := range workloadReceiverKinds {
		if kind != receiver {
			others = append(others, kind)
		}
	}
	return others
}

// pruneWorkloadTraceSignals reclaims the trace signals of this workload's own
// receiver identities. An identity is the receiver provider plus the exact
// runtime project, and the project alone is not a complete key: the Tempo
// provider registers <provider>:<project> signal sources of its own, so the
// prune removes trace signals only under the receiver's provider key as well.
// A same-ID registration owned by another provider or application keeps its
// signals. Only the project the reconciliation belongs to is affected: a
// shared receiver instance, another application's registrations and this
// application's other environments all keep their trace verification. The
// metrics and log signals of the reclaimed identities stay registered, which
// is what a receiver transition or a disabled trace policy must leave behind.
func pruneWorkloadTraceSignals(project string, receivers []capability.ProviderKind) error {
	for _, receiver := range receivers {
		if err := observability.PruneProviderSignals(fmt.Sprintf("%s:%s", receiver, project), receiver, observability.SignalTraces); err != nil {
			return err
		}
	}
	return nil
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
