package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

// RegistryConflictError names the poisoned (application_id, environment)
// group and every colliding deployment id, so every surface renders the same
// structured, actionable conflict (0427-175). The message carries the VERIFIED
// repair path (hp4 BF-10 / lesson 162): a poisoned twin cannot be reached by
// NAME (the name selector hits the same inventory seam), so the operator
// addresses the colliding records by --uuid.
type RegistryConflictError struct {
	Target        string
	Key           DeploymentIdentity
	DeploymentIDs []string
}

func (e *RegistryConflictError) Error() string {
	parts := make([]string, 0, len(e.DeploymentIDs))
	for _, id := range e.DeploymentIDs {
		parts = append(parts, "'baha app destroy --uuid "+id+" --full-reset'")
	}
	return ErrRegistryConflict.Error() +
		": multiple deployments claim application_id " + e.Key.ApplicationID +
		" environment " + e.Key.Environment + " on target " + e.Target +
		": " + strings.Join(e.DeploymentIDs, ", ") +
		"; every planning command on the Target fails closed until the duplicate is retired; repair each colliding deployment by uuid (" +
		strings.Join(parts, " then ") + "), then verify with 'baha app list'"
}

// Unwrap exposes the sentinel so errors.Is classifies the conflict.
func (e *RegistryConflictError) Unwrap() error { return ErrRegistryConflict }

// IsRegistryConflict classifies the poisoned-registry conflict across the
// storage seam boundary, where the plain sentinel string of
// ErrRegistryConflict is the stable carrier (0427-175).
func IsRegistryConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrRegistryConflict.Error())
}

// RegistryConflictIDs extracts the colliding deployment ids from a
// registry-conflict error so the command surface can name every record that
// must be retired by --uuid during recovery (0427-175). It reads the
// structured type through errors.As and falls back to the canonical message
// tail after "repair: baha app destroy --uuid ", so it works across every
// wrapping the machine layer performs.
func RegistryConflictIDs(err error) []string {
	if err == nil {
		return nil
	}
	var conflict *RegistryConflictError
	if errors.As(err, &conflict) && len(conflict.DeploymentIDs) > 0 {
		return append([]string(nil), conflict.DeploymentIDs...)
	}
	if !IsRegistryConflict(err) {
		return nil
	}
	const idsMarker = " on target "
	msg := err.Error()
	start := strings.Index(msg, idsMarker)
	if start < 0 {
		return nil
	}
	rest := msg[start+len(idsMarker):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return nil
	}
	rest = rest[colon+1:]
	if repair := strings.Index(rest, ";"); repair >= 0 {
		rest = rest[:repair]
	}
	fields := strings.Split(rest, ",")
	var ids []string
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			ids = append(ids, field)
		}
	}
	return ids
}

// RegistryConflictKey reports the (application_id, environment) pair whose
// group collided, from the same canonical message (0427-175).
func RegistryConflictKey(err error) (applicationID, environment string, ok bool) {
	if !IsRegistryConflict(err) {
		return "", "", false
	}
	msg := err.Error()
	if start := strings.Index(msg, "application_id "); start >= 0 {
		rest := msg[start+len("application_id "):]
		if end := strings.IndexByte(rest, ' '); end >= 0 {
			applicationID = rest[:end]
			rest = rest[end+len(" environment "):]
			if end := strings.IndexByte(rest, ' '); end >= 0 {
				environment = rest[:end]
				return applicationID, environment, true
			}
		}
	}
	return "", "", false
}

// ErrDeploymentOwnerAccessDenied is returned when the Core installation under a
// Target cannot be READ at all — a permission-denied or other OS-level access
// failure on the installation.json — rather than being present but foreign. A
// missing or unreadable installation is a host-state access problem, not a
// foreign-owner condition: the rule never reads a file it cannot open, so it
// fails closed on this specific error and never masquerades a missing file as
// "foreign installation". The wrapped fs error carries the path and cause.
var ErrDeploymentOwnerAccessDenied = errors.New("deployment state directory for its Target is not readable; the Core installation.json is not accessible to this user — check owner and permissions before its state is loadable")

// ErrDeploymentOwnerMalformedState is returned when the Core installation under
// a Target LOADS but is itself malformed or unclassifiable (invalid JSON, a
// missing mandatory capability, inconsistent readiness, or a bad selection).
// The one-installation-owner rule only proves foreignness from a POSITIVE
// loaded identity whose Target diverges; it never proves foreignness from a
// state it cannot even validate. A malformed state is therefore its own
// fail-closed named error, never masqueraded as a foreign installation.
var ErrDeploymentOwnerMalformedState = errors.New("deployment state directory for its Target holds a Core installation that could not be validated; the installation.json is present but malformed or incompatible — restore a genuine Core installation before its state is loadable")

// RefuseApplicationIDConflictOnDisplay is the exported detector the command
// planning guards run over the tolerant display listing (0427-175): the exact
// production rule, same sentinel, surviving the damage it detects.
func RefuseApplicationIDConflictOnDisplay(target string, records []DeploymentRecord) error {
	return refuseApplicationIDCollisions(target, records)
}

// refuseApplicationIDCollisions fails closed when more than one deployment
// record of target claims the same (application_id, environment) pair, naming
// the application_id, the environment and every colliding deployment id so the
// operator can reach the repair (0427-175). The message states the exact
// working command for the collision this registry can actually contain.
func refuseApplicationIDCollisions(target string, records []DeploymentRecord) error {
	byKey := map[DeploymentIdentity][]DeploymentRecord{}
	for i := range records {
		id := records[i].Identity
		if id.Target != target || strings.TrimSpace(id.ApplicationID) == "" {
			continue
		}
		key := DeploymentIdentity{Target: id.Target, ApplicationID: id.ApplicationID, Environment: id.Environment}
		byKey[key] = append(byKey[key], records[i])
	}
	for key, group := range byKey {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(a, b int) bool { return group[a].Identity.DeploymentID < group[b].Identity.DeploymentID })
		ids := make([]string, 0, len(group))
		for _, record := range group {
			ids = append(ids, record.Identity.DeploymentID)
		}
		return &RegistryConflictError{Target: target, Key: key, DeploymentIDs: ids}
	}
	return nil
}

func DeleteDeploymentRecord(id DeploymentIdentity) error {
	root, err := DeploymentRoot(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(root)
}

// refuseForeignDeploymentClaim protects the persisted bytes at path against an
// identity-mismatched overwrite. A readable deployment.json that belongs to a
// DIFFERENT deployment uuid is a foreign claim and the write fails closed
// before anything is replaced (0427-175). A missing file, a corrupt file or an
// unsupported version is deliberately not classified here: the one-writer guard
// read above owns those failure paths and already fails closed on them.
func refuseForeignDeploymentClaim(id DeploymentIdentity, path string) error {
	existing, exists, err := persistedDeploymentIdentity(path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if existing.DeploymentID == "" || existing.DeploymentID == id.DeploymentID {
		return nil
	}
	return &DeploymentRecordStateError{
		Identity: id,
		Kind:     "conflict",
		Err: fmt.Errorf("%w: the deployment path for %s/%s/%s already carries the record of deployment %s; a write never replaces another identity's record — address the existing deployment by uuid (--uuid) instead",
			ErrDuplicateApplicationID,
			id.Target, id.Application, id.Environment, existing.DeploymentID),
	}
}

// persistedDeploymentIdentity reads ONLY the identity of the deployment record
// persisted at path, for the write-seam foreign-claim check. A missing file is
// not a claim (returns exists=false, nil); an unparseable or version-broken
// file is reported as an unclaimable foreign occupant (exists=true) because no
// identity can be established for it and an overwrite would destroy evidence.
func persistedDeploymentIdentity(path string) (existing DeploymentIdentity, exists bool, err error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return DeploymentIdentity{}, false, nil
		}
		return DeploymentIdentity{}, false, readErr
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Version != DeploymentRecordVersion {
		return DeploymentIdentity{}, true, nil
	}
	return record.Identity, true, nil
}

// findDeploymentMatches lists the deployment registry for target and returns a
// copy of every record whose (application_id, environment) equals the request.
// A registry that already holds more than one such record is a pre-existing
// conflict; the caller reports it instead of silently picking one. The loop is
// extracted here so the name-identity uniqueness guard can reuse the exact same
// identity scan without duplicating the filter or the ambiguity report.
func findDeploymentMatches(target, applicationID, environment string) ([]DeploymentRecord, error) {
	records, err := ListDeployments(target)
	if err != nil {
		return nil, err
	}
	var matches []DeploymentRecord
	for i := range records {
		record := records[i]
		if record.Identity.ApplicationID != applicationID || record.Identity.Environment != environment {
			continue
		}
		matches = append(matches, record)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf(
			"multiple deployments claim application_id %s environment %s on target %s",
			applicationID, environment, target,
		)
	}
	return matches, nil
}

// normalisePersistedOwner mirrors Selection.Normalize for a persisted
// reconciliation owner read from a previous deployment record: it trims and
// lower-cases the value, and collapses any value that is neither "" nor a
// recognised owner (baseharbor|external) — a corrupt or unknown persisted owner
// — to "" (unset). This keeps the one-writer guard from either silently blocking
// (an unknown owner that is neither direct- nor delegated-legal) or silently
// attesting an owner that the validate() switch would otherwise reject, so the
// absent/corrupt value defers to the applied selection's mode.
func normalisePersistedOwner(owner string) string {
	normalised := delivery.ReconciliationOwner(strings.ToLower(strings.TrimSpace(owner)))
	if normalised != delivery.OwnerBaseHarbor && normalised != delivery.OwnerExternal {
		return ""
	}
	return string(normalised)
}

// refuseOneWriterMutation enforces ADR 0022 §2 at the apply path: exactly one
// active reconciliation owner per managed resource set. It runs after the
// applied selection's single-record validation and before any state is written.
//
// It observes the deployment's own observed state: a recorded conflict (two
// active owners for the same resource set) fails closed, and a direct
// (BaseHarbor-owned) mutation is refused while an external reconciler already
// owns the resource. An absent observed state (direct mode, no external owner)
// is the normal happy path and passes.
func refuseOneWriterMutation(record DeploymentRecord) error {
	applied := record.Applied.Delivery
	observed := record.Observed

	// Conflict observation wins closed: two active owners for one resource set.
	if observed.Conflict {
		return ErrConflictingOneWriter
	}
	// Direct mutation while an external reconciler already owns the resource.
	if applied.Mode == delivery.ModeDirect && observed.Owner == delivery.OwnerExternal {
		return ErrUnauthorizedMutation
	}
	return nil
}
