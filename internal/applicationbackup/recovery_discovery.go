package applicationbackup

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

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
			Support:         RecoverySupported,
			DefaultSelected: true,
			Durable:         true,
		})
	}
	if application.HasIdentity(m) {
		placement, placementErr := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
		if placementErr != nil {
			return RecoverySelection{}, placementErr
		}
		contributor := RecoveryContributor{
			StateClass:      StateIdentity,
			LogicalResource: "application-identity",
			Ownership:       "application",
			Support:         RecoveryUnsupported,
			Durable:         true,
			Reason:          "portable OIDC client and authentication policy are reconstructed from application.metadata, but provider-held users, credentials, MFA and passkey state do not yet have a scoped recovery export",
		}
		if placement.Scope == capability.ScopeExternal {
			contributor.Ownership = "external"
			contributor.Support = RecoveryExternal
			contributor.Durable = false
			contributor.ExplicitlyExcluded = true
			contributor.Reason = "external identity-directory state remains outside BaseHarbor recovery ownership; portable OIDC binding intent is reconstructed and re-verified"
		}
		contributors = append(contributors, contributor)
	}
	if application.HasLogsCollection(m) {
		placement, placementErr := application.ResolveProviderPlacement(m, capability.ProviderLoki)
		if placementErr != nil {
			return RecoverySelection{}, placementErr
		}
		contributor := RecoveryContributor{
			StateClass:      StateLogs,
			LogicalResource: "application",
			Ownership:       "application",
			Support:         RecoverySupported,
			DefaultSelected: false,
			Reason:          "application log history is selectable operational history and is excluded by default",
		}
		if placement.Scope == capability.ScopeExternal {
			contributor.Ownership = "external"
			contributor.Support = RecoveryExternal
			contributor.DefaultSelected = false
			contributor.ExplicitlyExcluded = true
			contributor.Reason = "external log history remains outside BaseHarbor recovery ownership"
		}
		contributors = append(contributors, contributor)
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

	contributors = append(contributors, RecoveryContributor{
		StateClass:      StatePKI,
		LogicalResource: "runtime-identities",
		Ownership:       "application",
		Support:         RecoverySupported,
		DefaultSelected: true,
		Reason:          "ephemeral runtime identities and trust edges are reconstructed from desired state; private CA keys are not copied",
	})

	return NewRecoverySelection(contributors)
}
