package hostresource

import (
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"strings"
)

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

// EstimateControlPlane accounts for every service in the shipped HA topology.
// These per-role planning budgets are estimates, not measured RSS baselines.
func EstimateControlPlane() (MemoryEstimate, error) {
	names, err := bhruntime.ControlPlaneStartupServices()
	if err != nil {
		return MemoryEstimate{}, err
	}
	var components []ComponentEstimate
	for _, name := range names {
		var mib uint64
		switch {
		case strings.HasPrefix(name, "postgres-member-"):
			mib = 512
		case strings.HasPrefix(name, "postgres-etcd-"):
			mib = 192
		case strings.HasPrefix(name, "openbao-member-"):
			mib = 384
		case name == "postgres" || name == "openbao":
			mib = 64
		case name == "postgres-admin" || name == "openbao-admin":
			mib = 64
		case name == "postgres-init":
			mib = 128
		default:
			return MemoryEstimate{}, fmt.Errorf("control-plane service %s has no resource planning budget", name)
		}
		component := heuristic(name, mib)
		component.Source = "topology-aware startup budget; not measured; includes bootstrap/admin services"
		components = append(components, component)
	}
	return Sum(components), nil
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

	if application.HasExplicitWorkload(m) {
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
