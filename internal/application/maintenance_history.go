package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MaintenanceHistoryFileName is the bounded, append-only history of durable
// maintenance operations one deployment has committed. The current window stays
// in MaintenanceAuthorityFileName so readers keep the fast path; the history is
// what makes a replay at an older revision answerable from durable state instead
// of from whatever the window happens to hold now.
const MaintenanceHistoryFileName = "maintenance-authority-history.json"

// MaintenanceHistoryLimit bounds the retained history the same way other
// protected deployment receipts are bounded.
const MaintenanceHistoryLimit = 4 << 20

// maintenanceHistoryFile resolves the protected history path with the same
// ownership rules as the window record.
func maintenanceHistoryFile(stateRoot string) (string, error) {
	if !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot {
		return "", errors.New("maintenance history requires protected absolute deployment state")
	}
	info, err := os.Lstat(stateRoot)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("maintenance history deployment state must remain owner-only")
	}
	return filepath.Join(stateRoot, MaintenanceHistoryFileName), nil
}

// readMaintenanceHistory returns the durable window history, oldest first. An
// absent file is an empty history, never an error.
func readMaintenanceHistory(stateRoot string) ([]MaintenanceAuthorityRecord, error) {
	path, err := maintenanceHistoryFile(stateRoot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read maintenance window history: %w", err)
	}
	if len(data) > MaintenanceHistoryLimit {
		return nil, errors.New("maintenance window history exceeds protected state limit")
	}
	var records []MaintenanceAuthorityRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode maintenance window history: %w", err)
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("validate maintenance window history: %w", err)
		}
	}
	return records, nil
}

// appendMaintenanceHistory durably records one committed operation. Rewriting the
// history is never allowed: entries must arrive in increasing revision order.
func appendMaintenanceHistory(stateRoot string, submitted MaintenanceAuthorityRecord) error {
	path, err := maintenanceHistoryFile(stateRoot)
	if err != nil {
		return err
	}
	records, err := readMaintenanceHistory(stateRoot)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		last := records[len(records)-1]
		if submitted.Revision <= last.Revision {
			return errors.New("maintenance window history cannot be rewritten")
		}
		if last.ApplicationID != submitted.ApplicationID || last.DeploymentID != submitted.DeploymentID {
			return ErrMaintenanceAuthorityForeign
		}
	}
	records = append(records, submitted)
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode maintenance window history: %w", err)
	}
	if len(data) > MaintenanceHistoryLimit {
		return errors.New("maintenance window history exceeds protected state limit")
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write maintenance window history: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect maintenance window history: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit maintenance window history: %w", err)
	}
	return nil
}

// loadMaintenanceRevision finds the durable operation a deployment committed at
// one revision. The retained window answers for the revision it currently holds;
// older revisions are answered from history, which is the only place that can
// still know them.
func loadMaintenanceRevision(stateRoot string, revision int) (MaintenanceAuthorityRecord, bool, error) {
	current, exists, err := LoadMaintenanceAuthority(stateRoot)
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	if exists && current.Revision == revision {
		return current, true, nil
	}
	records, err := readMaintenanceHistory(stateRoot)
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	for _, record := range records {
		if record.Revision == revision {
			return record, true, nil
		}
	}
	return MaintenanceAuthorityRecord{}, false, nil
}
