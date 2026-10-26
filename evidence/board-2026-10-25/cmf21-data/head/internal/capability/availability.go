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
	native := func(count int, limits string) availability.Support {
		return availability.Support{Level: availability.PartiallySupported, RecommendedInstances: count, Limits: limits, Guarantees: availability.Guarantees{MemberFailureTolerance: true, HostFailureTolerance: false, FailureDomain: "runtime-host"}}
	}
	switch kind {
	case ProviderPostgreSQL:
		return native(3, "native shared PostgreSQL uses Patroni and an odd authenticated etcd quorum on one runtime host; application-scoped PostgreSQL remains single-instance"), nil
	case ProviderValkey:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 3,
			Limits:               "verified Sentinel-backed member/process failure tolerance on the selected runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	case ProviderOpenBao:
		return native(3, "native Core OpenBao HA uses three service members and the retained shared PostgreSQL backend; a single Core installation cannot be silently converted"), nil
	case ProviderCaddy:
		return unsupported("single-host exposure owns one loopback HTTPS port; stable routing and config/TLS hot reload are supported, but redundant ingress/member-failure tolerance is not"), nil
	case ProviderSeaweedFS:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 3,
			Limits:               "verified three-member SeaweedFS member/process failure tolerance and stable S3/Admin continuity on one runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				CredentialRotation:     true,
				PKIRotation:            true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	case ProviderOTelCollector:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 2,
			Limits:               "verified redundant OpenTelemetry Collector member/process failure tolerance and stable OTLP endpoint continuity on one runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				PKIRotation:            true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	case ProviderExternalOTLP:
		return unsupported("external OTLP availability must be explicitly declared and verified by the selected external provider"), nil
	case ProviderPrometheus:
		return availability.Support{
			Level:                availability.Supported,
			RecommendedInstances: 2,
			Limits:               "verified redundant Prometheus member/process failure tolerance and stable query/management continuity on one runtime host; host-failure tolerance requires a multi-host runtime",
			Guarantees: availability.Guarantees{
				MemberFailureTolerance: true,
				HostFailureTolerance:   false,
				RollingMaintenance:     true,
				ManagementContinuity:   true,
				CredentialRotation:     true,
				PKIRotation:            true,
				FailureDomain:          "runtime-host",
			},
		}, nil
	case ProviderLoki:
		return native(3, "native Loki uses three service members and explicitly requested replicated platform object storage on one runtime host"), nil
	case ProviderTempo:
		return native(2, "native Tempo uses fixed replicated live-store partitions and redundant distributors/query services with platform object storage on one runtime host"), nil
	case ProviderKeycloak:
		return native(3, "native Keycloak uses three identity members and a Patroni/etcd PostgreSQL backend on one runtime host"), nil
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
