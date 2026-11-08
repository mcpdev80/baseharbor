package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

// MaintenanceTombstoneIdentityForTest exposes the production identity
// derivation for probes and regressions.
func MaintenanceTombstoneIdentityForTest(stateRoot string) (string, string, bool) {
	return maintenanceTombstoneIdentity(stateRoot)
}

// LoadMaintenanceRevision is the production revision lookup exported for
// durable replay: it answers which maintenance operation one deployment
// committed at one revision from the retained window, its history, or — after
// a destroy tore the deployment root down — the committed tombstone. An absent
// revision is an empty answer; a malformed, foreign or otherwise invalid
// durable record fails closed.
func LoadMaintenanceRevision(stateRoot string, revision int) (MaintenanceAuthorityRecord, bool, error) {
	return loadMaintenanceRevision(stateRoot, revision)
}

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
// still know them. When the deployment root is gone — the destroy owner tears it
// down — the committed destroy tombstone answers the destroyed window's
// revision: this is the production replay reader for the durable destroy
// record, keyed by the Target and deployment identity the root names.
func loadMaintenanceRevision(stateRoot string, revision int) (MaintenanceAuthorityRecord, bool, error) {
	current, exists, err := LoadMaintenanceAuthority(stateRoot)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return MaintenanceAuthorityRecord{}, false, err
		}
		// The deployment root itself is gone (a torn-down destroy): the window
		// is unreadable. Only an installation-owned root answers from its
		// durable tombstone.
		return maintenanceTombstoneRevision(stateRoot, revision)
	}
	if exists && current.Revision == revision {
		return current, true, nil
	}
	records, err := readMaintenanceHistory(stateRoot)
	if err != nil {
		// An unreadable history beneath a root that no longer exists (a
		// torn-down destroy) is answered from the durable tombstone; anything
		// else is a genuine error and fails closed.
		if isMissingStateRootErr(err) {
			return maintenanceTombstoneRevision(stateRoot, revision)
		}
		return MaintenanceAuthorityRecord{}, false, err
	}
	for _, record := range records {
		if record.Revision == revision {
			return record, true, nil
		}
	}
	// Nothing beneath the root knows this revision. A torn-down or rolled-back
	// root answers from the durable tombstone; a live window that simply never
	// committed this revision stays an honest "unknown" — never a tombstone the
	// root's own history does not contain.
	if !exists {
		return maintenanceTombstoneRevision(stateRoot, revision)
	}
	return MaintenanceAuthorityRecord{}, false, nil
}

// isMissingStateRootErr reports that a protected-state read failed because the
// deployment state root (or a file beneath it) is gone — the exact condition a
// destroyed deployment leaves behind, where only the durable tombstone can
// still answer.
func isMissingStateRootErr(err error) bool {
	var pathErr *os.PathError
	return errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOENT)
}

// maintenanceTombstoneReader resolves the durable destroy tombstone path for
// one (target, deploymentID) identity, or "" when none can be derived. The
// client registers the real resolver so this package can consume the tombstone
// the destroy route stamps outside the deployment root. A test that replaces
// the reader with nothing proves the replay fallback is wired through the
// production lookup: without it the torn-down root answers unknown.
var maintenanceTombstoneReader func(target, deploymentID string) string

// maintenanceTombstoneOwnedRoot reports whether a state root names a
// deployment this installation owns, in the shape
// <data-root>/targets/<target>/deployments/<deployment-id>/state. The fallback
// applies ONLY to such roots: an arbitrary caller path must never be answered
// from a tombstone, or the lookup would read unrelated maintenance records as
// durable destroy receipts.
var maintenanceTombstoneOwnedRoot func(stateRoot, target, deploymentID string) bool

// SetMaintenanceTombstoneReplay wires the production destroy-tombstone replay
// reader into the revision lookup. The client package calls it once at init:
// the tombstone is written by the destroy route and read here, so the durable
// outcome stays queryable after the owner removes the deployment root.
func SetMaintenanceTombstoneReplay(resolvePath func(target, deploymentID string) string,
	ownedRoot func(stateRoot, target, deploymentID string) bool) {
	maintenanceTombstoneReader = resolvePath
	maintenanceTombstoneOwnedRoot = ownedRoot
}

// maintenanceTombstoneIdentity derives the (target, deploymentID) pair a
// deployment state root names. Production has two managed layouts: the
// repository-local state root <data-root>/targets/<target>/applications/<id>
// and the managed deployment state root
// <data-root>/targets/<target>/deployments/<deployment-id>. Roots outside the
// managed layout answer false.
func maintenanceTombstoneIdentity(stateRoot string) (string, string, bool) {
	clean := filepath.Clean(stateRoot)
	dir, leaf := filepath.Split(clean)
	if leaf == "" || leaf == string(filepath.Separator) {
		return "", "", false
	}
	parent, namespace := filepath.Split(filepath.Clean(dir))
	var target string
	switch namespace {
	case "applications", "deployments":
		_, target = filepath.Split(filepath.Clean(parent))
	default:
		// <data-root>/targets/<target>/state is not a destroy-able deployment
		// namespace: no tombstone identity derives from it.
		if leaf != "state" {
			return "", "", false
		}
		if _, name := filepath.Split(filepath.Clean(dir)); name != "targets" {
			return "", "", false
		}
		_, target = filepath.Split(filepath.Clean(filepath.Dir(filepath.Clean(dir))))
	}
	if target == "" || target == "." {
		return "", "", false
	}
	return target, leaf, true
}

// maintenanceTombstoneRevision answers one revision from the durable destroy
// tombstone. An absent tombstone (or an unowned/unresolvable root) is an
// empty answer, never an error; a malformed, oversized, foreign or otherwise
// invalid tombstone fails closed so a replay can never adopt a record the
// installation cannot stand behind.
//
// Order is part of the security binding: ownership and identity are refused
// BEFORE the tombstone file is addressed, so an arbitrary caller path can never
// make this lookup read a maintenance record it does not own. The registered
// reader itself is what production wires; a test replaces the reader (not the
// guards) to prove the replay path is load-bearing.
func maintenanceTombstoneRevision(stateRoot string, revision int) (MaintenanceAuthorityRecord, bool, error) {
	empty, absent := MaintenanceAuthorityRecord{}, false
	resolvePath := maintenanceTombstoneReader
	ownedRoot := maintenanceTombstoneOwnedRoot
	if resolvePath == nil || ownedRoot == nil {
		// No production route can consume a tombstone: an explicit fail-closed
		// error, never a silent "unknown", because the destroy writer would
		// otherwise leave a durable record nothing reads.
		return empty, absent, errors.New("maintenance destroy tombstone has no production replay reader")
	}
	target, deploymentID, ok := maintenanceTombstoneIdentity(stateRoot)
	if !ok {
		// The root names no derivable deployment identity; nothing to consult.
		return empty, absent, nil
	}
	if !ownedRoot(stateRoot, target, deploymentID) {
		return empty, absent, nil
	}
	path := resolvePath(target, deploymentID)
	if path == "" {
		return empty, absent, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, absent, nil
	}
	if err != nil {
		return empty, absent, fmt.Errorf("read maintenance tombstone: %w", err)
	}
	if len(data) > MaintenanceAuthorityLimit {
		return empty, absent, errors.New("maintenance tombstone exceeds protected state limit")
	}
	var record MaintenanceAuthorityRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return empty, absent, fmt.Errorf("decode maintenance tombstone: %w", err)
	}
	if err := record.Validate(); err != nil {
		return empty, absent, fmt.Errorf("validate maintenance tombstone: %w", err)
	}
	if record.Target != target || record.DeploymentID != deploymentID {
		return empty, absent, ErrMaintenanceAuthorityForeign
	}
	if record.Revision != revision {
		return empty, absent, nil
	}
	return record, true, nil
}
