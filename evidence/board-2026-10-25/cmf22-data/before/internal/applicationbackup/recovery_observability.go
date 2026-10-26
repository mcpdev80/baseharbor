package applicationbackup

import (
	"encoding/json"
	"fmt"

	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
)

const logsHistoryEntry = "observability/logs.json"

func LogsHistoryPayloadEntry(backup logsprovider.HistoryBackup) (PayloadEntry, error) {
	data, err := json.Marshal(backup)
	if err != nil {
		return PayloadEntry{}, fmt.Errorf("encode log-history recovery payload: %w", err)
	}
	return PayloadEntry{Name: logsHistoryEntry, Data: data}, nil
}

func LogsHistoryFromPayload(payload Payload) (logsprovider.HistoryBackup, bool, error) {
	for _, entry := range payload.Entries {
		if entry.Name != logsHistoryEntry {
			continue
		}
		var backup logsprovider.HistoryBackup
		if err := json.Unmarshal(entry.Data, &backup); err != nil {
			return logsprovider.HistoryBackup{}, true, fmt.Errorf("decode log-history recovery payload: %w", err)
		}
		return backup, true, nil
	}
	return logsprovider.HistoryBackup{}, false, nil
}
