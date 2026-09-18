package capability

// Reference providers describe the capability products already used by the
// current Compose implementation. They are descriptors only: existing
// provisioning remains authoritative while the shared capability and provider
// registry layers describe portable intent, placement and ownership.
var (
	PostgreSQL = Provider{
		Kind:         ProviderPostgreSQL,
		Capabilities: []Kind{SQL},
	}
	Valkey = Provider{
		Kind:         ProviderValkey,
		Capabilities: []Kind{KeyValue},
	}
	OpenBao = Provider{
		Kind:         ProviderOpenBao,
		Capabilities: []Kind{Secrets},
	}
)
