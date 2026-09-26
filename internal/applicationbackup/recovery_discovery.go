package applicationbackup

import "github.com/mcpdev80/baseharbor/internal/application"

func DiscoverManifestRecovery(m application.Manifest) (RecoverySelection, error) {
	contributors := []RecoveryContributor{{
		StateClass:      StateApplicationMetadata,
		Ownership:       "application",
		Support:         RecoverySupported,
		DefaultSelected: true,
		Durable:         true,
	}}

	for _, instance := range application.SQLInstanceNames(m) {
		contributors = append(contributors, RecoveryContributor{
			StateClass:      StateSQL,
			LogicalResource: instance,
			Ownership:       "application",
			Support:         RecoverySupported,
			DefaultSelected: true,
			Durable:         true,
		})
	}
	if m.Services.Secrets {
		contributors = append(contributors, RecoveryContributor{
			StateClass:      StateSecrets,
			LogicalResource: "default",
			Ownership:       "application",
			Support:         RecoverySupported,
			DefaultSelected: true,
			Durable:         true,
		})
	}
	for _, bucket := range application.ObjectStorageBucketNames(m) {
		contributors = append(contributors, RecoveryContributor{
			StateClass:      StateObjectStorage,
			LogicalResource: bucket,
			Ownership:       "application",
			Support:         RecoveryUnsupported,
			Durable:         true,
			Reason:          "object-storage capture and restore are not implemented yet",
		})
	}
	if application.HasLogsCollection(m) {
		for _, source := range m.Logs.Collect {
			contributors = append(contributors, RecoveryContributor{
				StateClass:      StateLogs,
				LogicalResource: source,
				Ownership:       "application",
				Support:         RecoveryUnsupported,
				Reason:          "scoped log-history recovery is not implemented yet",
			})
		}
	}
	if application.HasMetricsSources(m) {
		for _, source := range m.Metrics.Sources {
			contributors = append(contributors, RecoveryContributor{
				StateClass:      StateMetrics,
				LogicalResource: source.Name,
				Ownership:       "application",
				Support:         RecoveryUnsupported,
				Reason:          "scoped metrics-history recovery is not implemented yet",
			})
		}
	}
	if application.HasOTLPTelemetry(m) {
		for _, signal := range m.Telemetry.OTLP.Signals {
			if signal != "traces" {
				continue
			}
			contributors = append(contributors, RecoveryContributor{
				StateClass:      StateTraces,
				LogicalResource: "default",
				Ownership:       "application",
				Support:         RecoveryUnsupported,
				Reason:          "scoped trace-history recovery is not implemented yet",
			})
			break
		}
	}

	return NewRecoverySelection(contributors)
}
