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
	Tempo         = Provider{Kind: ProviderTempo, Capabilities: []Kind{Traces}}
	Keycloak      = Provider{Kind: ProviderKeycloak, Capabilities: []Kind{Identity}}
	ExternalOIDC  = Provider{Kind: ProviderExternalOIDC, Capabilities: []Kind{Identity}}
)

var (
	PostgreSQLIntegration = IntegrationDescriptor{
		ID: "baseharbor/postgresql", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: PostgreSQL,
		Services:        []ServiceKind{ServiceSQL},
		Capabilities:    []SpecificationID{SQLV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "sql", Class: InterfaceApplication, Protocol: "postgresql", Intrinsic: true}, {Name: "management-ui", Class: InterfaceAdministration, Protocol: "https", Optional: true}},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "metrics", Kind: ObservabilityMetrics, Status: ObservabilityRequiresAdapter, Mode: ObservabilityAdapter, Protocol: "openmetrics", Verification: ObservabilityVerifyNone},
			{Name: "logs", Kind: ObservabilityLogs, Status: ObservabilitySupported, Mode: ObservabilityRuntime, Protocol: "stdout-stderr", SemanticConvention: "baseharbor.runtime.logs", Verification: ObservabilityVerifyBackend},
			{Name: "traces", Kind: ObservabilityTraces, Status: ObservabilitySupported, Mode: ObservabilityInteraction, Protocol: "interaction", SemanticConvention: "database", Verification: ObservabilityVerifySpan},
		}},
	}
	ValkeyIntegration = IntegrationDescriptor{
		ID: "baseharbor/valkey", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Valkey,
		Services:        []ServiceKind{ServiceCache},
		Capabilities:    []SpecificationID{KeyValueV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "cache", Class: InterfaceApplication, Protocol: "resp", Intrinsic: true}, {Name: "management-ui", Class: InterfaceAdministration, Protocol: "https", Optional: true}},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "metrics", Kind: ObservabilityMetrics, Status: ObservabilityRequiresAdapter, Mode: ObservabilityAdapter, Protocol: "openmetrics", Verification: ObservabilityVerifyNone},
			{Name: "logs", Kind: ObservabilityLogs, Status: ObservabilitySupported, Mode: ObservabilityRuntime, Protocol: "stdout-stderr", SemanticConvention: "baseharbor.runtime.logs", Verification: ObservabilityVerifyBackend},
			{Name: "traces", Kind: ObservabilityTraces, Status: ObservabilitySupported, Mode: ObservabilityInteraction, Protocol: "interaction", SemanticConvention: "database", Verification: ObservabilityVerifySpan},
		}},
	}
	OpenBaoIntegration = IntegrationDescriptor{
		ID: "baseharbor/openbao", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: OpenBao,
		Services:        []ServiceKind{ServiceSecrets},
		Capabilities:    []SpecificationID{SecretsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Interfaces: []ProviderInterface{{Name: "api", Class: InterfaceManagement, Protocol: "https", Intrinsic: true}, {Name: "management-ui", Class: InterfaceAdministration, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Backup: true, Restore: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "metrics", Kind: ObservabilityMetrics, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "traces", Kind: ObservabilityTraces, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
		}},
	}
	CaddyIntegration = IntegrationDescriptor{
		ID: "baseharbor/caddy", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Caddy,
		Services:        []ServiceKind{ServiceExposure},
		Capabilities:    []SpecificationID{ExposureHTTPV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "ingress", Class: InterfaceApplication, Protocol: "http", Intrinsic: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "metrics", Kind: ObservabilityMetrics, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "traces", Kind: ObservabilityTraces, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
		}},
	}
	SeaweedFSIntegration = IntegrationDescriptor{
		ID: "baseharbor/seaweedfs", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: SeaweedFS,
		Services:        []ServiceKind{ServiceObjectStorage},
		Capabilities:    []SpecificationID{ObjectStorageS3V1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Interfaces: []ProviderInterface{{Name: "s3", Class: InterfaceApplication, Protocol: "s3", Intrinsic: true}, {Name: "management-ui", Class: InterfaceAdministration, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "metrics", Kind: ObservabilityMetrics, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "traces", Kind: ObservabilityTraces, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
		}},
	}
	OTelCollectorIntegration = IntegrationDescriptor{
		ID: "baseharbor/opentelemetry-collector", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: OTelCollector,
		Services:        []ServiceKind{ServiceObservability},
		Capabilities:    []SpecificationID{TelemetryOTLPV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Interfaces: []ProviderInterface{{Name: "otlp", Class: InterfaceApplication, Protocol: "otlp", Intrinsic: true}, {Name: "health", Class: InterfaceHealth, Protocol: "http", Intrinsic: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "collector-metrics", Kind: ObservabilityMetrics, Status: ObservabilitySupported, Mode: ObservabilityNative, Protocol: "openmetrics", Verification: ObservabilityVerifyBackend, Port: 8888, Path: "/metrics"},
			{Name: "collector-logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "collector-traces", Kind: ObservabilityTraces, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
		}},
	}
	PrometheusIntegration = IntegrationDescriptor{
		ID: "baseharbor/prometheus", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Prometheus,
		Services:        []ServiceKind{ServiceObservability},
		Capabilities:    []SpecificationID{MetricsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "query-api", Class: InterfaceObservability, Protocol: "https", Intrinsic: true}, {Name: "management-ui", Class: InterfaceObservability, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "prometheus-metrics", Kind: ObservabilityMetrics, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "prometheus-logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "prometheus-traces", Kind: ObservabilityTraces, Status: ObservabilityNotApplicable, Verification: ObservabilityVerifyNone},
		}},
	}
	LokiIntegration = IntegrationDescriptor{
		ID: "baseharbor/loki", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Loki,
		Services:        []ServiceKind{ServiceObservability},
		Capabilities:    []SpecificationID{LogsV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "query-api", Class: InterfaceObservability, Protocol: "https", Intrinsic: true}, {Name: "visualization-ui", Class: InterfaceObservability, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "loki-metrics", Kind: ObservabilityMetrics, Status: ObservabilitySupported, Mode: ObservabilityNative, Protocol: "openmetrics", Verification: ObservabilityVerifyBackend, Port: 3100, Path: "/metrics"},
			{Name: "loki-logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "loki-traces", Kind: ObservabilityTraces, Status: ObservabilityNotApplicable, Verification: ObservabilityVerifyNone},
		}},
	}
	TempoIntegration = IntegrationDescriptor{
		ID: "baseharbor/tempo", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Tempo,
		Services:        []ServiceKind{ServiceObservability},
		Capabilities:    []SpecificationID{TracesV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared},
		Interfaces: []ProviderInterface{{Name: "query-api", Class: InterfaceObservability, Protocol: "https", Intrinsic: true}, {Name: "visualization-ui", Class: InterfaceObservability, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "tempo-metrics", Kind: ObservabilityMetrics, Status: ObservabilitySupported, Mode: ObservabilityNative, Protocol: "openmetrics", Verification: ObservabilityVerifyBackend, Port: 3200, Path: "/metrics"},
			{Name: "tempo-logs", Kind: ObservabilityLogs, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
			{Name: "tempo-traces", Kind: ObservabilityTraces, Status: ObservabilityNotApplicable, Verification: ObservabilityVerifyNone},
		}},
	}
	KeycloakIntegration = IntegrationDescriptor{
		ID: "baseharbor/keycloak", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: Keycloak,
		Services:        []ServiceKind{ServiceIdentity},
		Capabilities:    []SpecificationID{IdentityOIDCV1.ID},
		SupportedScopes: []ProviderScope{ScopeShared, ScopeApplication},
		Interfaces: []ProviderInterface{{Name: "oidc", Class: InterfaceApplication, Protocol: "oidc", Intrinsic: true}, {Name: "login-account", Class: InterfaceUserFacing, Protocol: "https", Intrinsic: true}, {Name: "admin-api", Class: InterfaceManagement, Protocol: "https", Intrinsic: true}, {Name: "management-ui", Class: InterfaceAdministration, Protocol: "https", Optional: true}},
		Optional:        OptionalLifecycleSupport{Status: true, Update: true, Destroy: true},
		Observability: ProviderObservability{Signals: []ProviderObservabilitySignal{
			{Name: "keycloak-metrics", Kind: ObservabilityMetrics, Status: ObservabilitySupported, Mode: ObservabilityNative, Protocol: "openmetrics", Verification: ObservabilityVerifyBackend, Port: 9000, Path: "/metrics"},
			{Name: "keycloak-logs", Kind: ObservabilityLogs, Status: ObservabilitySupported, Mode: ObservabilityRuntime, Protocol: "stdout-stderr", SemanticConvention: "baseharbor.runtime.logs", Verification: ObservabilityVerifyBackend},
			{Name: "keycloak-traces", Kind: ObservabilityTraces, Status: ObservabilityUnsupported, Verification: ObservabilityVerifyNone},
		}},
	}
	ExternalOIDCIntegration = IntegrationDescriptor{
		ID: "baseharbor/external-oidc", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: ExternalOIDC,
		Services:        []ServiceKind{ServiceIdentity},
		Capabilities:    []SpecificationID{IdentityOIDCV1.ID},
		SupportedScopes: []ProviderScope{ScopeExternal},
		Interfaces: []ProviderInterface{{Name: "oidc", Class: InterfaceApplication, Protocol: "oidc", Intrinsic: true}, {Name: "login-account", Class: InterfaceUserFacing, Protocol: "https", Intrinsic: true}},
	}
	ExternalOTLPIntegration = IntegrationDescriptor{
		ID: "baseharbor/external-otlp", Version: "0.1.0",
		Protocol: ProviderProtocolV1, Provider: ExternalOTLP,
		Services:        []ServiceKind{ServiceObservability},
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
	case ProviderTempo:
		return TempoIntegration, nil
	case ProviderKeycloak:
		return KeycloakIntegration, nil
	case ProviderExternalOIDC:
		return ExternalOIDCIntegration, nil
	default:
		return IntegrationDescriptor{}, fmt.Errorf("reference integration for provider %q is not defined", provider)
	}
}
