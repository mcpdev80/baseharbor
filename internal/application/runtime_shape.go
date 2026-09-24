package application

// HasManagedRuntimeServices reports whether BaseHarbor owns a materialized
// PostgreSQL or Valkey runtime for the application. Shared object storage uses
// its own lazy provider runtime and is intentionally not part of this project.
func HasManagedRuntimeServices(m Manifest) bool {
	return len(SQLInstanceNames(m)) > 0 || len(CacheInstanceNames(m)) > 0
}

// HasObjectStorage reports whether the application explicitly requests at least
// one logical S3 bucket.
func HasObjectStorage(m Manifest) bool {
	return len(ObjectStorageBucketNames(m)) > 0
}

// HasExplicitWorkload reports whether the repository manifest explicitly
// declares a Compose workload. Workload-only applications must be explicit so
// validation never guesses based on files outside the manifest.
func HasExplicitWorkload(m Manifest) bool {
	return m.Workload.Compose != "" || len(m.Workload.Services) > 0
}
