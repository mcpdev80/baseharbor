package capability

// ProviderChangeContract describes what the existing provider declaration can
// serve and which inventory its adapter must observe. It is not runtime usage,
// placement, a new registration mechanism or permission to mutate.
type ProviderChangeContract struct {
	Provider      ProviderKind `json:"provider"`
	Functions     []string     `json:"functions"`
	MaterialKinds []string     `json:"material_kinds"`
	Aspects       []string     `json:"aspects"`
}

// ChangeContract projects lifecycle analysis metadata for the shipped catalog.
// Unknown extensions remain visibly unsupported, never generically safe.
func (d IntegrationDescriptor) ChangeContract() (ProviderChangeContract, bool) {
	c := ProviderChangeContract{Provider: d.Provider.Kind}
	switch d.Provider.Kind {
	case ProviderPostgreSQL:
		c.Functions = []string{"sql.transactions", "sql.queries"}
		c.MaterialKinds = []string{"database", "schema", "wal", "credentials", "backup"}
		c.Aspects = []string{"transactions", "consistency", "durability", "protocol", "availability", "reconnection", "recovery"}
	case ProviderValkey:
		c.Functions = []string{"cache.access", "key-value.durable", "session.access", "job.state"}
		c.MaterialKinds = []string{"cache", "durable-key-value", "session", "job", "snapshot", "aof"}
		c.Aspects = []string{"durability", "expiry", "eviction", "consistency", "reconnection", "recovery"}
	case ProviderOpenBao:
		c.Functions = []string{"secrets.delivery", "secrets.policy", "credentials.issue", "credentials.renew", "credentials.revoke", "pki.issue", "pki.renew", "pki.revoke", "pki.validate"}
		c.MaterialKinds = []string{"secret", "policy", "lease", "key", "certificate", "recovery"}
		c.Aspects = []string{"permissions", "encryption", "delivery", "lease-expiry", "revocation", "renewal", "availability", "recovery"}
	case ProviderCaddy:
		c.Functions = []string{"http.routing", "tls.termination", "pki.renew"}
		c.MaterialKinds = []string{"route", "configuration", "key", "certificate"}
		c.Aspects = []string{"dns", "routing", "tls", "renewal", "availability", "reconnection"}
	case ProviderSeaweedFS:
		c.Functions = []string{"objects.read", "objects.write"}
		c.MaterialKinds = []string{"object", "file", "metadata", "credentials", "backup"}
		c.Aspects = []string{"durability", "consistency", "s3-compatibility", "permissions", "retention", "availability", "recovery"}
	case ProviderOTelCollector, ProviderExternalOTLP:
		c.Functions = []string{"telemetry.receive", "telemetry.forward"}
		c.MaterialKinds = []string{"telemetry-buffer", "configuration"}
		c.Aspects = []string{"delivery", "buffer-loss", "routing", "protocol", "retention", "reconnection"}
	case ProviderPrometheus:
		c.Functions = []string{"metrics.ingest", "metrics.query", "metrics.rules"}
		c.MaterialKinds = []string{"metrics", "wal", "rule", "configuration"}
		c.Aspects = []string{"retention", "scrape-gap", "query-access", "availability", "recovery"}
	case ProviderLoki:
		c.Functions = []string{"logs.ingest", "logs.query"}
		c.MaterialKinds = []string{"logs", "wal", "index", "configuration"}
		c.Aspects = []string{"retention", "delivery", "query-access", "availability", "recovery"}
	case ProviderTempo:
		c.Functions = []string{"traces.ingest", "traces.query"}
		c.MaterialKinds = []string{"traces", "wal", "index", "configuration"}
		c.Aspects = []string{"retention", "delivery", "query-access", "availability", "recovery"}
	case ProviderKeycloak, ProviderExternalOIDC:
		c.Functions = []string{"identity.login", "identity.sso", "identity.issue", "identity.validate", "identity.jwks", "identity.session", "identity.refresh"}
		c.MaterialKinds = []string{"user", "realm", "client", "session", "signing-key", "token", "configuration", "backup"}
		c.Aspects = []string{"authentication", "authorization", "token-expiry", "jwks-refresh", "session-expiry", "reconnection", "availability", "recovery"}
	case ProviderRabbitMQ:
		c.Functions = []string{"queue.publish", "queue.consume", "pubsub.delivery", "stream.read", "stream.write"}
		c.MaterialKinds = []string{"queue", "message", "in-flight", "stream", "offset", "topology", "credentials"}
		c.Aspects = []string{"ordering", "delivery", "acknowledgement", "durability", "protocol", "reconnection", "recovery"}
	case ProviderRuntimeBroker:
		c.Functions = []string{"runtime.authorize", "runtime.credentials", "runtime.policy"}
		c.MaterialKinds = []string{"policy", "credentials", "configuration", "certificate", "recovery"}
		c.Aspects = []string{"authorization", "credential-renewal", "tls", "availability", "reconnection", "recovery"}
	case ProviderRuntimeExecutor:
		c.Functions = []string{"runtime.execute", "runtime.observe", "runtime.control"}
		c.MaterialKinds = []string{"in-flight", "configuration", "certificate", "credentials"}
		c.Aspects = []string{"execution", "in-flight-work", "authorization", "tls", "availability", "reconnection"}
	case ProviderMongoDB:
		c.Functions = []string{"document.read", "document.write", "document.transactions"}
		c.MaterialKinds = []string{"document", "index", "oplog", "credentials", "backup"}
		c.Aspects = []string{"transactions", "consistency", "durability", "protocol", "availability", "reconnection", "recovery"}
	default:
		return c, false
	}
	return c, true
}
