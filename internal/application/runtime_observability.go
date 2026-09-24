package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func reconcileManagedRuntimeObservability(m Manifest) error {
	logsPolicy, err := LogsPolicy(m)
	if err != nil {
		return err
	}
	logsEnabled := logsPolicy.Enabled && logsPolicy.Collect[LogsSourceApplicationProvider]
	project := RuntimeProjectName(m)

	if err := reconcileRuntimeProviderObservability(m, project, "postgresql", capability.ProviderPostgreSQL, capability.PostgreSQLIntegration, SQLInstanceNames(m), logsEnabled); err != nil {
		return err
	}
	return reconcileRuntimeProviderObservability(m, project, "valkey", capability.ProviderValkey, capability.ValkeyIntegration, CacheInstanceNames(m), logsEnabled)
}

func reconcileRuntimeProviderObservability(
	m Manifest,
	project string,
	serviceKind string,
	provider capability.ProviderKind,
	descriptor capability.IntegrationDescriptor,
	instances []string,
	logsEnabled bool,
) error {
	keep := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		service := runtimeServiceName(serviceKind, instance)
		id := fmt.Sprintf("%s:%s:%s", provider, project, service)
		keep[id] = struct{}{}

		enabled := map[observability.SignalKind]bool{observability.SignalLogs: logsEnabled}
		signals := map[string]observability.ProviderSignalRuntime{}
		if logsEnabled {
			signals["logs"] = observability.ProviderSignalRuntime{
				Target: observability.RuntimeTarget(project, service),
			}
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
