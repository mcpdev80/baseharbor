package capability

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/availability"
)

// AvailabilitySupportForProvider classifies every shipped provider realization.
// HA capability is claimed only when BaseHarbor has a real realization and
// verification path. Product capability alone is never sufficient.
func AvailabilitySupportForProvider(kind ProviderKind) (availability.Support, error) {
	unsupported := func(reason string) availability.Support {
		return availability.Support{Level: availability.Unsupported, Limits: reason}
	}
	switch kind {
	case ProviderPostgreSQL:
		return unsupported("current BaseHarbor PostgreSQL reference realization is single-instance"), nil
	case ProviderValkey:
		return unsupported("current BaseHarbor Valkey reference realization is single-instance"), nil
	case ProviderOpenBao:
		return unsupported("current BaseHarbor OpenBao reference realization uses one managed server"), nil
	case ProviderCaddy:
		return unsupported("current BaseHarbor exposure reference realization has no redundant runtime topology"), nil
	case ProviderSeaweedFS:
		return unsupported("current BaseHarbor SeaweedFS reference realization does not verify HA topology"), nil
	case ProviderOTelCollector:
		return unsupported("current BaseHarbor OTLP collector reference realization is single-instance"), nil
	case ProviderExternalOTLP:
		return unsupported("external OTLP availability must be explicitly declared and verified by the selected external provider"), nil
	case ProviderPrometheus:
		return unsupported("current BaseHarbor Prometheus reference realization is single-instance"), nil
	case ProviderLoki:
		return unsupported("current BaseHarbor Loki reference realization is single-instance"), nil
	case ProviderTempo:
		return unsupported("current BaseHarbor Tempo reference realization is single-instance"), nil
	case ProviderKeycloak:
		return unsupported("current BaseHarbor Keycloak reference realization is single-instance"), nil
	case ProviderExternalOIDC:
		return unsupported("external OIDC availability must be explicitly declared and verified by the selected external provider"), nil
	case ProviderRabbitMQ:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 3,
			Limits:               "verified member/process failure tolerance on the selected runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	case ProviderMongoDB:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 3,
			Limits:               "verified replica-set member/process failure tolerance on the selected runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	default:
		return availability.Support{}, fmt.Errorf("availability classification for provider %q is not defined", kind)
	}
}
