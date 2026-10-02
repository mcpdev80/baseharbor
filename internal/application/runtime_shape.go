package application

// HasManagedRuntimeServices reports whether BaseHarbor owns a materialized
// PostgreSQL, Valkey, RabbitMQ or MongoDB runtime for the application. Shared object storage uses
// its own lazy provider runtime and is intentionally not part of this project.
func HasManagedRuntimeServices(m Manifest) bool {
	return len(SQLInstanceNames(m)) > 0 || len(ValkeyInstanceNames(m)) > 0 || len(RabbitMQInstanceNames(m)) > 0 || len(DocumentDatabaseInstanceNames(m)) > 0
}

// HasObjectStorage reports whether the application explicitly requests at least
// one logical S3 bucket.
func HasObjectStorage(m Manifest) bool {
	return len(ObjectStorageBucketNames(m)) > 0
}

// HasExplicitWorkload reports whether portable application intent declares
// logical workload components. Source-format metadata is not required here.
func HasExplicitWorkload(m Manifest) bool {
	return len(m.Workload.Components) > 0
}
