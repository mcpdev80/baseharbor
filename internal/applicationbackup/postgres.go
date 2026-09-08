package applicationbackup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

const postgresEntryPrefix = "postgres/"
const postgresEntrySuffix = ".sql"

func PostgresPayloadEntries(backups []application.PostgresBackup) ([]PayloadEntry, error) {
	entries := make([]PayloadEntry, 0, len(backups))
	seen := make(map[string]struct{}, len(backups))
	for _, backup := range backups {
		if strings.TrimSpace(backup.Instance) == "" {
			return nil, fmt.Errorf("postgres backup instance is required")
		}
		name := postgresEntryPrefix + backup.Instance + postgresEntrySuffix
		if !validLogicalName(name) {
			return nil, fmt.Errorf("invalid postgres backup instance %q", backup.Instance)
		}
		if _, duplicate := seen[backup.Instance]; duplicate {
			return nil, fmt.Errorf("duplicate postgres backup instance %q", backup.Instance)
		}
		seen[backup.Instance] = struct{}{}
		entries = append(entries, PayloadEntry{Name: name, Data: append([]byte(nil), backup.SQL...)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func PostgresBackupsFromPayload(m application.Manifest, payload Payload) ([]application.PostgresBackup, error) {
	backups := make([]application.PostgresBackup, 0, len(application.PostgresInstanceNames(m)))
	for _, entry := range payload.Entries {
		if !strings.HasPrefix(entry.Name, postgresEntryPrefix) {
			continue
		}
		base := strings.TrimPrefix(entry.Name, postgresEntryPrefix)
		if !strings.HasSuffix(base, postgresEntrySuffix) {
			return nil, fmt.Errorf("unsupported postgres backup entry %q", entry.Name)
		}
		instance := strings.TrimSuffix(base, postgresEntrySuffix)
		if instance == "" || strings.Contains(instance, "/") {
			return nil, fmt.Errorf("invalid postgres backup entry %q", entry.Name)
		}
		backups = append(backups, application.PostgresBackup{Instance: instance, SQL: append([]byte(nil), entry.Data...)})
	}
	if err := application.ValidatePostgresBackupSet(m, backups); err != nil {
		return nil, err
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Instance < backups[j].Instance })
	return backups, nil
}
