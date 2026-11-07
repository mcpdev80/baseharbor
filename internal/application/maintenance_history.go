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
		return fmt.Errorf("%w: encode maintenance window history: %v", ErrMaintenanceHistoryAppendFailed, err)
	}
	if len(data) > MaintenanceHistoryLimit {
		return fmt.Errorf("%w: maintenance window history exceeds protected state limit", ErrMaintenanceHistoryAppendFailed)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%w: write maintenance window history: %v", ErrMaintenanceHistoryAppendFailed, err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: protect maintenance window history: %v", ErrMaintenanceHistoryAppendFailed, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: commit maintenance window history: %v", ErrMaintenanceHistoryAppendFailed, err)
	}
	return nil
}

// ErrMaintenanceHistoryAppendFailed is the named, terminal error a failed history
// append returns so a caller can treat it as fail-closed: no advanced current
// window may survive a missing historical revision.
var ErrMaintenanceHistoryAppendFailed = errors.New("maintenance window append failed")

// rollbackMaintenanceHistory removes the last retained history entry only when it
// is the entry the failed commit appended (identified by revision). A failed
// append leaves that entry on disk even though the current window rolls back; a
// destroy that fails therefore has to remove both writes as one transaction or a
// retry answers the aborted revision as already recorded. Production points it at
// the real removal; a test can make the removal a no-op to prove the fix is
// load-bearing.
var rollbackMaintenanceHistory = rollbackMaintenanceHistoryImpl

var RollbackMaintenanceHistoryHook = rollbackMaintenanceHistory

func rollbackMaintenanceHistoryImpl(stateRoot string, appendedRevision int) error {
	records, err := readMaintenanceHistory(stateRoot)
	if err != nil {
		return err
	}
	// No history file at all: nothing appended to remove.
	if len(records) == 0 {
		return nil
	}
	last := records[len(records)-1]
	// Only the entry the failed commit appended (its revision) is removed. An
	// entry from an earlier, durable revision is never touched.
	if last.Revision != appendedRevision {
		return nil
	}
	trimmed := records[:len(records)-1]
	path, err := maintenanceHistoryFile(stateRoot)
	if err != nil {
		return err
	}
	if len(trimmed) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove appended maintenance history: %w", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(trimmed, "", "  ")
	if err != nil {
		return fmt.Errorf("encode maintenance window history (rollback): %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write maintenance window history (rollback): %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect maintenance window history (rollback): %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit maintenance window history (rollback): %w", err)
	}
	return nil
}

// --- test seams ---
// These small exported helpers let round-2 tests exercise the production
// transaction (history-append rollback, revision answerability, capacity
// preflight) through their real code paths. They are thin wrappers, not
// re-implementations.

// LoadMaintenanceRevisionForTest is the production revision-lookup so a test can
// prove an older revision stays answerable after recovery.
func LoadMaintenanceRevisionForTest(stateRoot string, revision int) (MaintenanceAuthorityRecord, bool, error) {
	return loadMaintenanceRevision(stateRoot, revision)
}

// MaintenanceStateFilePathForTest resolves the current-window path so a test can
// advance the current window while deliberately failing the history append.
func MaintenanceStateFilePathForTest(stateRoot string) (string, error) {
	return maintenanceStateFile(stateRoot)
}

// TestMaintenanceHistoryHasCapacityForTest is the production capacity preflight so
// a test can force the irreversible commit to fail before it advances.
func TestMaintenanceHistoryHasCapacityForTest(stateRoot string, current MaintenanceAuthorityRecord) error {
	return maintenanceHistoryHasCapacity(stateRoot, current)
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
