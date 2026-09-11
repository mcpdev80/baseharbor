package application

// HasManagedRuntimeServices reports whether BaseHarbor owns a materialized
// PostgreSQL or Valkey runtime for the application. Managed secrets currently
// require one of those services and therefore do not form a standalone runtime.
func HasManagedRuntimeServices(m Manifest) bool {
	return len(PostgresInstanceNames(m)) > 0 || len(RedisInstanceNames(m)) > 0
}

// HasExplicitWorkload reports whether the repository manifest explicitly
// declares a Compose workload. Workload-only applications must be explicit so
// validation never guesses based on files outside the manifest.
func HasExplicitWorkload(m Manifest) bool {
	return m.Workload.Compose != "" || len(m.Workload.Services) > 0
}
