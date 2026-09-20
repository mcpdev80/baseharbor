package capability

import "fmt"

// Reference providers describe the capability products already used by the
// current Compose implementation. They are descriptors only: existing
// provisioning remains authoritative while the shared capability and provider
// registry layers describe portable intent, placement and ownership.
var (
	PostgreSQL    = Provider{Kind: ProviderPostgreSQL, Capabilities: []Kind{SQL}}
	Valkey        = Provider{Kind: ProviderValkey, Capabilities: []Kind{KeyValue}}
	OpenBao       = Provider{Kind: ProviderOpenBao, Capabilities: []Kind{Secrets}}
	Caddy         = Provider{Kind: ProviderCaddy, Capabilities: []Kind{ExposureHTTP}}
	SeaweedFS     = Provider{Kind: ProviderSeaweedFS, Capabilities: []Kind{ObjectStorageS3}}
	OTelCollector = Provider{Kind: ProviderOTelCollector, Capabilities: []Kind{TelemetryOTLP}}
	ExternalOTLP  = Provider{Kind: ProviderExternalOTLP, Capabilities: []Kind{TelemetryOTLP}}
	Prometheus    = Provider{Kind: ProviderPrometheus, Capabilities: []Kind{Metrics}}
	Loki          = Provider{Kind: ProviderLoki, Capabilities: []Kind{Logs}}
)

var (
	PostgreSQLIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: PostgreSQL,
		Capabilities:    []SpecificationID{SQLV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
	}
	ValkeyIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: Valkey,
		Capabilities:    []SpecificationID{KeyValueV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
	}
	OpenBaoIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: OpenBao,
		Capabilities:    []SpecificationID{SecretsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Backup: true, Restore: true, Destroy: true},
	}
	CaddyIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: Caddy,
		Capabilities:    []SpecificationID{ExposureHTTPV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
	}
	SeaweedFSIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: SeaweedFS,
		Capabilities:    []SpecificationID{ObjectStorageS3V1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
	}
	OTelCollectorIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: OTelCollector,
		Capabilities:    []SpecificationID{TelemetryOTLPV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
	}
	PrometheusIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: Prometheus,
		Capabilities:    []SpecificationID{MetricsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
	}
	LokiIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: Loki,
		Capabilities:    []SpecificationID{LogsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
	}
	ExternalOTLPIntegration = IntegrationDescriptor{
		Protocol: ProviderProtocolV1, Provider: ExternalOTLP,
		Capabilities:    []SpecificationID{TelemetryOTLPV1.ID},
		SupportedScopes: []ProviderScope{ScopeExternal},
	}
)

func ReferenceIntegration(provider ProviderKind) (IntegrationDescriptor, error) {
	switch provider {
	case ProviderPostgreSQL:
		return PostgreSQLIntegration, nil
	case ProviderValkey:
		return ValkeyIntegration, nil
	case ProviderOpenBao:
		return OpenBaoIntegration, nil
	case ProviderCaddy:
		return CaddyIntegration, nil
	case ProviderSeaweedFS:
		return SeaweedFSIntegration, nil
	case ProviderOTelCollector:
		return OTelCollectorIntegration, nil
	case ProviderExternalOTLP:
		return ExternalOTLPIntegration, nil
	case ProviderPrometheus:
		return PrometheusIntegration, nil
	case ProviderLoki:
		return LokiIntegration, nil
	default:
		return IntegrationDescriptor{}, fmt.Errorf("reference integration for provider %q is not defined", provider)
	}
}
