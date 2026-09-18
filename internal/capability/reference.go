package capability

// Reference providers describe the capability products already used by the
// current Compose implementation. They are descriptors only: existing
// provisioning remains authoritative until it is migrated behind the shared
// lifecycle in later v0.4.1 commits.
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
