package application

// PostgresBackupTarget describes one managed PostgreSQL instance using only
// stable runtime identifiers required by the backup/restore layer.
type PostgresBackupTarget struct {
	Instance string
	Service  string
	Database string
}

func PostgresBackupTargets(m Manifest) []PostgresBackupTarget {
	instances := PostgresInstanceNames(m)
	targets := make([]PostgresBackupTarget, 0, len(instances))
	for _, instance := range instances {
		targets = append(targets, PostgresBackupTarget{
			Instance: instance,
			Service:  runtimeServiceName("postgres", instance),
			Database: postgresDatabaseName(m, instance),
		})
	}
	return targets
}
