package applicationbackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
)

const (
	objectStorageEntryPrefix   = "object-storage/"
	objectStorageEntrySuffix   = ".json"
	workloadStorageEntryPrefix = "workload-storage/"
	workloadStorageEntrySuffix = ".tar"
)

func ObjectStoragePayloadEntry(backup objectstorage.BucketBackup) (PayloadEntry, error) {
	logical := strings.TrimSpace(backup.LogicalBucket)
	if logical == "" || strings.ContainsAny(logical, "/\\") {
		return PayloadEntry{}, errors.New("object-storage backup logical bucket is invalid")
	}
	data, err := json.Marshal(backup)
	if err != nil {
		return PayloadEntry{}, fmt.Errorf("encode object-storage backup %s: %w", logical, err)
	}
	return PayloadEntry{Name: objectStorageEntryPrefix + logical + objectStorageEntrySuffix, Data: data}, nil
}

func ObjectStorageBackupsFromPayload(payload Payload) ([]objectstorage.BucketBackup, error) {
	var result []objectstorage.BucketBackup
	seen := map[string]struct{}{}
	for _, entry := range payload.Entries {
		if !strings.HasPrefix(entry.Name, objectStorageEntryPrefix) {
			continue
		}
		base := strings.TrimPrefix(entry.Name, objectStorageEntryPrefix)
		if !strings.HasSuffix(base, objectStorageEntrySuffix) {
			return nil, fmt.Errorf("unsupported object-storage backup entry %q", entry.Name)
		}
		logical := strings.TrimSuffix(base, objectStorageEntrySuffix)
		if logical == "" || strings.ContainsAny(logical, "/\\") {
			return nil, fmt.Errorf("invalid object-storage backup entry %q", entry.Name)
		}
		if _, ok := seen[logical]; ok {
			return nil, fmt.Errorf("duplicate object-storage backup %q", logical)
		}
		var backup objectstorage.BucketBackup
		if err := json.Unmarshal(entry.Data, &backup); err != nil {
			return nil, fmt.Errorf("decode object-storage backup %s: %w", logical, err)
		}
		if backup.LogicalBucket != logical {
			return nil, fmt.Errorf("object-storage backup entry %q identity mismatch", entry.Name)
		}
		seen[logical] = struct{}{}
		result = append(result, backup)
	}
	return result, nil
}

func WorkloadStoragePayloadEntry(volume string, archive []byte) (PayloadEntry, error) {
	volume = strings.TrimSpace(volume)
	if volume == "" || strings.ContainsAny(volume, "/\\") {
		return PayloadEntry{}, errors.New("workload storage volume name is invalid")
	}
	return PayloadEntry{Name: workloadStorageEntryPrefix + volume + workloadStorageEntrySuffix, Data: append([]byte(nil), archive...)}, nil
}

func WorkloadStorageFromPayload(payload Payload) (map[string][]byte, error) {
	result := map[string][]byte{}
	for _, entry := range payload.Entries {
		if !strings.HasPrefix(entry.Name, workloadStorageEntryPrefix) {
			continue
		}
		base := strings.TrimPrefix(entry.Name, workloadStorageEntryPrefix)
		if !strings.HasSuffix(base, workloadStorageEntrySuffix) {
			return nil, fmt.Errorf("unsupported workload-storage backup entry %q", entry.Name)
		}
		volume := strings.TrimSuffix(base, workloadStorageEntrySuffix)
		if volume == "" || strings.ContainsAny(volume, "/\\") {
			return nil, fmt.Errorf("invalid workload-storage backup entry %q", entry.Name)
		}
		if _, ok := result[volume]; ok {
			return nil, fmt.Errorf("duplicate workload-storage backup %q", volume)
		}
		result[volume] = append([]byte(nil), entry.Data...)
	}
	return result, nil
}
