package applicationbackup

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

const openBaoSecretsEntry = "secrets/openbao.json"

func OpenBaoPayloadEntry(backup openbao.ApplicationSecretBackup) (PayloadEntry, error) {
	data, err := json.Marshal(backup)
	if err != nil {
		return PayloadEntry{}, fmt.Errorf("encode OpenBao application secret backup: %w", err)
	}
	return PayloadEntry{Name: openBaoSecretsEntry, Data: data}, nil
}

func OpenBaoBackupFromPayload(application, environment string, payload Payload) (openbao.ApplicationSecretBackup, error) {
	if payload.Manifest.Application != application || payload.Manifest.Environment != environment {
		return openbao.ApplicationSecretBackup{}, errors.New("backup archive identity does not match restore target")
	}
	var raw []byte
	for _, entry := range payload.Entries {
		if entry.Name != openBaoSecretsEntry {
			continue
		}
		if raw != nil {
			return openbao.ApplicationSecretBackup{}, errors.New("backup contains duplicate OpenBao secret entry")
		}
		raw = entry.Data
	}
	if raw == nil {
		return openbao.ApplicationSecretBackup{}, errors.New("backup does not contain OpenBao application secrets")
	}
	var backup openbao.ApplicationSecretBackup
	if err := json.Unmarshal(raw, &backup); err != nil {
		return openbao.ApplicationSecretBackup{}, errors.New("OpenBao application secret backup is malformed")
	}
	if backup.Identity.Name != application || backup.Identity.Environment != environment {
		return openbao.ApplicationSecretBackup{}, errors.New("OpenBao backup identity does not match restore target")
	}
	return backup, nil
}
