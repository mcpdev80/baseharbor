package hostresource

import "github.com/mcpdev80/baseharbor/internal/application"

const (
	MiB uint64 = 1 << 20
)

func baseline(name string, estimatedMiB uint64) ComponentEstimate {
	estimated := estimatedMiB * MiB
	return ComponentEstimate{
		Name:           name,
		MinimumBytes:   estimated / 2,
		EstimatedBytes: estimated,
		Confidence:     ConfidenceKnownBaseline,
		Source:         "BaseHarbor reference-provider calibration",
	}
}

func heuristic(name string, estimatedMiB uint64) ComponentEstimate {
	return ComponentEstimate{
		Name:           name,
		EstimatedBytes: estimatedMiB * MiB,
		Confidence:     ConfidenceEstimated,
		Source:         "conservative unbounded workload estimate",
	}
}

func EstimateControlPlane() MemoryEstimate {
	return Sum([]ComponentEstimate{
		baseline("control-plane PostgreSQL", 192),
		baseline("OpenBao", 192),
		baseline("runtime control", 64),
	})
}

func EstimateApplication(m application.Manifest) MemoryEstimate {
	var components []ComponentEstimate

	for range application.SQLInstanceNames(m) {
		components = append(components, baseline("SQL provider/resource", 256))
	}
	for range application.CacheInstanceNames(m) {
		components = append(components, baseline("cache provider/resource", 128))
	}
	if m.Services.ObjectStorage {
		components = append(components, baseline("object storage provider", 256))
	}
	if m.Services.Secrets {
		components = append(components, baseline("application secrets path", 64))
	}
	if m.Services.Identity {
		components = append(components,
			baseline("identity database", 192),
			baseline("identity provider", 768),
		)
	}
	if application.HasMetricsSources(m) {
		components = append(components, baseline("metrics provider", 256))
	}
	if application.HasLogsCollection(m) {
		components = append(components,
			baseline("logs provider", 256),
			baseline("log collector", 128),
		)
	}
	if application.HasOTLPTelemetry(m) {
		components = append(components, baseline("telemetry collector", 128))
	}
	if application.HasTraceSignal(m) {
		components = append(components, baseline("trace provider", 256))
	}
	if m.Services.SQLManagementUI {
		components = append(components, baseline("SQL management UI", 192))
	}
	if m.Services.CacheManagementUI {
		components = append(components, baseline("cache management UI", 128))
	}
	if m.Services.ObjectStorageManagementUI {
		components = append(components, baseline("object storage management UI", 128))
	}
	if m.Services.SecretsManagementUI {
		components = append(components, baseline("secrets management UI", 64))
	}
	if m.Services.IdentityManagementUI {
		components = append(components, baseline("identity management UI", 64))
	}
	if m.Services.ObservabilityManagementUI {
		components = append(components, baseline("observability management UI", 128))
	}

	if len(m.Workload.Services) > 0 || m.Workload.Compose != "" {
		components = append(components, heuristic("application workload", 256))
	}
	if len(m.Exposures) > 0 {
		components = append(components, baseline("developer/exposure gateway", 64))
	}
	if application.RequiresRuntimeBroker(m) {
		components = append(components, baseline("application runtime broker", 64))
	}
	return Sum(components)
}
