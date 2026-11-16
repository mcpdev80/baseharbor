package application

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

type RuntimeComponentObservability struct {
	ID               string
	Provider         capability.ProviderKind
	Class            observability.SourceClass
	Scope            capability.ProviderScope
	OwnerApplication string

	MetricsNetwork  string
	MetricsTarget   string
	MetricsPath     string
	MetricsSecurity observability.Security

	LogsTarget string

	TracesTarget   string
	TracesSecurity observability.Security
}

func ReconcileRuntimeComponentObservability(m Manifest, component RuntimeComponentObservability) error {
	component.ID = strings.TrimSpace(component.ID)
	if component.ID == "" || component.Provider == "" {
		return fmt.Errorf("runtime component observability requires id and provider")
	}

	metricsPolicy, err := MetricsPolicy(m)
	if err != nil {
		return err
	}
	logsPolicy, err := LogsPolicy(m)
	if err != nil {
		return err
	}
	tracesPolicy, err := TracesPolicy(m)
	if err != nil {
		return err
	}

	classEnabled := func(kind observability.SignalKind) bool {
		switch component.Class {
		case observability.SourceApplicationProvider:
			switch kind {
			case observability.SignalMetrics:
				return metricsPolicy.Enabled && metricsPolicy.Collect[MetricsSourceApplicationProvider]
			case observability.SignalLogs:
				return logsPolicy.Enabled && logsPolicy.Collect[LogsSourceApplicationProvider]
			case observability.SignalTraces:
				return tracesPolicy.Enabled && tracesPolicy.Collect[TracesSourceApplicationProvider]
			}
		case observability.SourcePlatformProvider:
			switch kind {
			case observability.SignalMetrics:
				return metricsPolicy.Enabled && metricsPolicy.Collect[MetricsSourcePlatformProvider]
			case observability.SignalLogs:
				return logsPolicy.Enabled && logsPolicy.Collect[LogsSourcePlatformProvider]
			case observability.SignalTraces:
				return tracesPolicy.Enabled && tracesPolicy.Collect[TracesSourcePlatformProvider]
			}
		}
		return false
	}

	sourceScope := func(kind observability.SignalKind) (capability.ProviderScope, string, bool, error) {
		if component.Class == observability.SourceApplicationProvider {
			return capability.ScopeApplication, "", true, nil
		}
		var provider capability.ProviderKind
		switch kind {
		case observability.SignalMetrics:
			provider = capability.ProviderPrometheus
		case observability.SignalLogs:
			provider = capability.ProviderLoki
		case observability.SignalTraces:
			provider = capability.ProviderTempo
		default:
			return "", "", false, fmt.Errorf("unsupported runtime observability signal kind %q", kind)
		}
		placement, err := ResolveProviderPlacement(m, provider)
		if err != nil {
			return "", "", false, err
		}
		if placement.Scope != capability.ScopeShared {
			return placement.Scope, placement.SharingBoundary, false, nil
		}
		return capability.ScopeShared, placement.SharingBoundary, true, nil
	}

	desired := make([]observability.SignalSource, 0, 3)
	if classEnabled(observability.SignalMetrics) && strings.TrimSpace(component.MetricsTarget) != "" {
		scope, boundary, allowed, err := sourceScope(observability.SignalMetrics)
		if err != nil {
			return err
		}
		if allowed {
			desired = append(desired, observability.SignalSource{
				ID:               component.ID,
				Kind:             observability.SignalMetrics,
				Provider:         component.Provider,
				Class:            component.Class,
				Scope:            scope,
				SharingBoundary:  boundary,
				OwnerApplication: component.OwnerApplication,
				Network:          strings.TrimSpace(component.MetricsNetwork),
				Target:           strings.TrimSpace(component.MetricsTarget),
				Protocol:         "openmetrics",
				Path:             strings.TrimSpace(component.MetricsPath),
				Mode:             capability.ObservabilityNative,
				Verification:     capability.ObservabilityVerifyBackend,
				Security:         component.MetricsSecurity,
			})
		}
	}
	if classEnabled(observability.SignalLogs) && strings.TrimSpace(component.LogsTarget) != "" {
		scope, boundary, allowed, err := sourceScope(observability.SignalLogs)
		if err != nil {
			return err
		}
		if allowed {
			desired = append(desired, observability.SignalSource{
				ID:                 component.ID,
				Kind:               observability.SignalLogs,
				Provider:           component.Provider,
				Class:              component.Class,
				Scope:              scope,
				SharingBoundary:    boundary,
				OwnerApplication:   component.OwnerApplication,
				Target:             strings.TrimSpace(component.LogsTarget),
				Protocol:           "stdout-stderr",
				Mode:               capability.ObservabilityRuntime,
				SemanticConvention: "baseharbor.runtime.logs",
				Verification:       capability.ObservabilityVerifyBackend,
			})
		}
	}
	if classEnabled(observability.SignalTraces) && strings.TrimSpace(component.TracesTarget) != "" {
		scope, boundary, allowed, err := sourceScope(observability.SignalTraces)
		if err != nil {
			return err
		}
		if allowed {
			desired = append(desired, observability.SignalSource{
				ID:                 component.ID,
				Kind:               observability.SignalTraces,
				Provider:           component.Provider,
				Class:              component.Class,
				Scope:              scope,
				SharingBoundary:    boundary,
				OwnerApplication:   component.OwnerApplication,
				Target:             strings.TrimSpace(component.TracesTarget),
				Protocol:           "otlp",
				Mode:               capability.ObservabilityNative,
				SemanticConvention: "http",
				Verification:       capability.ObservabilityVerifySpan,
				Security:           component.TracesSecurity,
			})
		}
	}

	return observability.ReconcileSignals(component.ID, desired)
}
