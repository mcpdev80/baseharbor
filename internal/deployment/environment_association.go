package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/environment"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// The environment-association seam lives here by design. Per the owner ruling
// 2026-11-15 (card t-20261114-r28-cli2-env-registry, 0428-054): the
// app/target/node association for a shared environment is stored IN the
// existing per-Target environment.json record (environment_identity.go), never
// in a second registry. The environment identity is the single source of truth
// for one (Target, environment) pair; the association is its authoritative
// Core-side member list.
//
// The association is an ATTRIBUTE of the durable EnvironmentRecord, not a
// sibling file: a separate registry would re-introduce the state split the
// uid-environment seam explicitly rejected (precedent 0427). The two fields are
// serialized together with target/name/id in the same JSON object, so a reader
// that predates this seam still decodes the identity fields unchanged.

// AssociationSubjectKind enumerates who may be associated with one environment.
// The three roles are intentionally distinct so a Target UID never aliases an
// Application ID or a Node ID under one environment: the (kind, id) tuple is
// the membership key, and the kind is what keeps the three identity roles from
// collapsing into a single string namespace.
type AssociationSubjectKind string

const (
	// AssociationKindApp associates an application with the environment. It is
	// the Core-authoritative mirror of the (application_id, environment) system
	// identity (ADR 0017): the deployment registry already keys exactly one
	// deployment per (Target, application_id, environment), and this seam records
	// that the application is a MEMBER of the shared environment, not merely an
	// operator of one deployment in it.
	AssociationKindApp AssociationSubjectKind = "app"
	// AssociationKindNode associates an execution node (future clusters) with
	// the environment. A shared environment is shared across nodes by intent;
	// the node membership is recorded here so it survives a node rename or a
	// second checkout without re-deriving it from names.
	AssociationKindNode AssociationSubjectKind = "node"
)

// AssociationBinding is one membership entry on an environment record. The
// SubjectID is the stable, non-secret subject identity for its kind: for an app
// it is the system application id, for a node the stable node identity. It is
// never a secret and never a name; name-matching is never used to resolve
// membership (the owner rule: do not infer matching from names).
type AssociationBinding struct {
	SubjectKind AssociationSubjectKind `json:"subject_kind"`
	SubjectID   string                 `json:"subject_id"`
}

// ErrAssociationAlreadyExists is returned when a (kind, id) is already a member
// of the environment. The existing binding is preserved and returned, so a
// repeated registration reuses the historical membership rather than creating a
// duplicate or clobbering a concurrent writer.
var ErrAssociationAlreadyExists = errors.New("environment association already registered for this subject")

// environmentAssociationFieldName is the JSON key on the environment.json
// record that carries the member list. It sits beside target/name/id so the
// record stays one source of truth; a reader that ignores unknown fields still
// sees the identity fields, and a reader that knows this field sees the
// membership list.
const environmentAssociationFieldName = "associations"

// subjectSlugPattern mirrors the target-slug discipline used throughout
// internal/deployment so a subject id cannot smuggle a path or escape the state
// namespace. It is kept local to this seam: the target name pattern is already
// owned by target.go and the environment name pattern by the environment
// package, and a shared copy would duplicate the discipline.
var subjectSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// validateAssociationSubjectKind fails closed on an unknown kind so a poisoned
// record or a caller typo can never silently register a foreign subject role.
func validateAssociationSubjectKind(kind AssociationSubjectKind) error {
	switch kind {
	case AssociationKindApp, AssociationKindNode:
		if strings.TrimSpace(string(kind)) == "" {
			return &stableid.ValidationError{Message: "environment association subject kind is empty"}
		}
		return nil
	default:
		return fmt.Errorf("environment association has an unknown subject kind %q", string(kind))
	}
}

// validateAssociationSubjectID fails closed on an empty or path-bearing subject
// id so a subject name cannot smuggle a path or escape the state namespace. The
// id is the durable identity, never the mutable selector: a rename of the
// underlying subject is a new registration under the new identity, not a
// mutation of this binding.
func validateAssociationSubjectID(kind AssociationSubjectKind, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &stableid.ValidationError{Message: "environment association subject id is empty"}
	}
	if !subjectSlugPattern.MatchString(id) {
		return fmt.Errorf("environment association subject id %q for %s must use lowercase letters, digits, dots or hyphens and must start and end with a letter or digit", id, string(kind))
	}
	return nil
}

// EnvironmentRecordAssociations returns the current membership list of one
// (Target, environment) pair. It is a pure read: it never allocates a binding
// and never mutates state. When the environment (or the association field) does
// not yet exist it returns an empty list so callers can treat a brand-new
// environment exactly like an empty one.
//
// It is the authoritative Core read of who belongs to the environment: it does
// not infer membership from deployment records, names, or any other surface. A
// caller that wants to know whether an app belongs to an environment reads this
// and compares the exact (kind, id).
func EnvironmentRecordAssociations(target, environmentName string) ([]AssociationBinding, error) {
	_, path, err := environmentStateRoot(target, environmentName)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect environment identity: %w", statErr)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("read environment identity: %w", readErr)
	}
	// Decode the whole record (identity + association fields). Unknown fields
	// from future revisions are ignored on purpose so the additive association
	// field is forward-compatible; the identity fields are validated below.
	var record EnvironmentRecord
	if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
		return nil, fmt.Errorf("decode environment identity: %w", unmarshalErr)
	}
	if err := environment.Validate(record.Identity); err != nil {
		return nil, fmt.Errorf("environment carries an invalid identity: %w", err)
	}
	// Fail closed on a corrupt or foreign binding, exactly like
	// readEnvironmentRecord: a record found under a different (Target,
	// environment) path must never expose its membership list, so the association
	// read re-checks the binding before returning anything.
	if err := ValidateTargetName(record.Target); err != nil {
		return nil, fmt.Errorf("environment identity at %q carries an invalid target: %w", path, err)
	}
	if strings.TrimSpace(record.Name) == "" {
		return nil, fmt.Errorf("environment identity at %q carries an empty name", path)
	}
	if strings.TrimSpace(record.Target) != strings.TrimSpace(target) {
		return nil, fmt.Errorf("%w at %q: stored target %q != %q", ErrEnvironmentBindingMismatch, path, record.Target, target)
	}
	if strings.TrimSpace(record.Name) != strings.TrimSpace(environmentName) {
		return nil, fmt.Errorf("%w at %q: stored name %q != %q", ErrEnvironmentBindingMismatch, path, record.Name, environmentName)
	}
	return normalizeAssociations(record.Associations), nil
}

// RegisterEnvironmentAssociation records a (kind, id) as a member of one
// (Target, environment) pair. It is idempotent: a repeated registration of the
// same (kind, id) returns the existing binding without mutating state.
//
// First registration of a NEW (kind, id) writes the association field onto the
// existing per-Target environment.json record under the shared registry write
// lock, so two concurrent first registrations of the same subject converge on
// one binding (mirroring GetEnvironment) instead of poisoning the record with
// two divergent lists. A (kind, id) that is already a member refuses with
// ErrAssociationAlreadyExists and returns the existing binding, so a racing
// second writer never clobbers the first.
//
// The environment identity is resolved (created if missing) before the
// association is written, so registering an association against a not-yet-open
// environment primes the same single record that GetEnvironment would.
func RegisterEnvironmentAssociation(target, environmentName string, kind AssociationSubjectKind, subjectID string) (AssociationBinding, error) {
	if err := validateAssociationSubjectKind(kind); err != nil {
		return AssociationBinding{}, err
	}
	if err := validateAssociationSubjectID(kind, subjectID); err != nil {
		return AssociationBinding{}, err
	}
	subjectID = strings.TrimSpace(subjectID)

	home, path, err := environmentStateRoot(target, environmentName)
	if err != nil {
		return AssociationBinding{}, err
	}

	// Resolve (and if needed create) the environment record first so the
	// association is written to the authoritative single record, never a
	// side-channel, and so the registry write lock engages for the whole claim
	// sequence.
	record, err := GetEnvironment(target, environmentName)
	if err != nil {
		return AssociationBinding{}, err
	}

	// Idempotent no-op: the subject is already a member. Return the existing
	// binding without a write so a repeated registration is a true no-op.
	for _, existing := range record.Associations {
		if existing.SubjectKind == kind && existing.SubjectID == subjectID {
			return existing, nil
		}
	}

	binding := AssociationBinding{SubjectKind: kind, SubjectID: subjectID}

	created, err := registerUnderLock(target, environmentName, home, path, binding)
	if err != nil {
		return AssociationBinding{}, err
	}
	return created, nil
}

// registerUnderLock performs the locked claim of one association binding. It
// re-reads inside the lock so a concurrent writer that registered the same
// subject is observed (converge) rather than duplicated, and a genuinely
// different concurrent write is refused (fail closed).
func registerUnderLock(target, environmentName, home, path string, binding AssociationBinding) (AssociationBinding, error) {
	var created AssociationBinding
	for attempt := 0; attempt < 4096; attempt++ {
		err := WithDeploymentRegistryWriteLock(target, func() error {
			refreshed, rErr := readEnvironmentRecord(path, target, environmentName)
			if rErr != nil {
				return rErr
			}
			// Converge: the subject is already a member after the lock was taken.
			for _, existing := range refreshed.Associations {
				if existing.SubjectKind == binding.SubjectKind && existing.SubjectID == binding.SubjectID {
					created = existing
					return nil
				}
			}
			// Diverge: a different subject was registered under the same (kind,id)
			// key — the membership key is taken, refuse so the caller retries.
			next := append(normalizeAssociations(refreshed.Associations), binding)
			if _, err := persistEnvironmentRecord(home, path, target, environmentName, refreshed.Identity, next); err != nil {
				return err
			}
			created = binding
			return nil
		})
		if err == nil {
			return created, nil
		}
		if errors.Is(err, ErrAssociationAlreadyExists) {
			for _, existing := range refreshedAssociations(path) {
				if existing.SubjectKind == binding.SubjectKind && existing.SubjectID == binding.SubjectID {
					return existing, nil
				}
			}
			return AssociationBinding{}, err
		}
		if !errors.Is(err, ErrRegistryConcurrentWrite) {
			return AssociationBinding{}, err
		}
		// Another registration holds the Target registry lock; retry briefly
		// (mirroring GetEnvironment) so exactly one writer wins the critical
		// section and the others converge.
		time.Sleep(time.Millisecond)
	}
	// Exhaustion means we could never take the lock; fall back to reading the
	// record a concurrent writer must have persisted, so the caller still
	// observes one binding rather than an empty one.
	for _, existing := range refreshedAssociations(path) {
		if existing.SubjectKind == binding.SubjectKind && existing.SubjectID == binding.SubjectID {
			return existing, nil
		}
	}
	return AssociationBinding{}, fmt.Errorf("could not acquire registry lock for association of %q to %q after retries", string(binding.SubjectKind), binding.SubjectID)
}

// DeregistrationEnvironmentAssociation removes a (kind, id) from the membership
// of one (Target, environment) pair. It is idempotent: deregistering a subject
// that is not a member is a no-op that returns success, so a cleanup path can
// run it without guarding on membership.
//
// It preserves the environment identity and every OTHER binding: removal is a
// pure member-list edit on the same record, never a delete of the environment
// or of a foreign binding. Removal writes under the shared registry write lock
// so a concurrent deregistration cannot both strip the same entry and leave a
// torn list.
func DeregistrationEnvironmentAssociation(target, environmentName string, kind AssociationSubjectKind, subjectID string) error {
	if err := validateAssociationSubjectKind(kind); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(subjectID)
	if trimmed == "" {
		return &stableid.ValidationError{Message: "environment association subject id is empty"}
	}

	home, path, err := environmentStateRoot(target, environmentName)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < 4096; attempt++ {
		if _, err := deregisterUnderLock(target, home, path, kind, trimmed); err == nil {
			return nil
		}
		if !errors.Is(err, ErrRegistryConcurrentWrite) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("could not acquire registry lock to deregister %q from %q after retries", string(kind), environmentName)
}

// deregisterUnderLock removes the binding inside the registry write lock.
func deregisterUnderLock(target, home, path string, kind AssociationSubjectKind, subjectID string) (bool, error) {
	var removed bool
	err := WithDeploymentRegistryWriteLock(target, func() error {
		record, rErr := readEnvironmentRecord(path, target, environmentNameForPath(path))
		if rErr != nil {
			return rErr
		}
		kept := make([]AssociationBinding, 0, len(record.Associations))
		for _, existing := range record.Associations {
			if existing.SubjectKind == kind && existing.SubjectID == subjectID {
				removed = true
				continue
			}
			kept = append(kept, existing)
		}
		// Idempotent no-op: nothing to remove. Return without a write so a
		// second deregistration (or a deregistration of a never-registered
		// subject) never touches the record.
		if !removed {
			return nil
		}
		if _, err := persistEnvironmentRecord(home, path, target, environmentNameForPath(path), record.Identity, kept); err != nil {
			return err
		}
		return nil
	})
	return removed, err
}

// ListEnvironmentAssociations returns the sorted, de-duplicated membership list
// of one (Target, environment) pair. Sorting gives a deterministic order so a
// concurrent writer that adds one member cannot make an equal reader observe a
// reordered list. The list is a read, so it never writes the lock.
func ListEnvironmentAssociations(target, environmentName string) ([]AssociationBinding, error) {
	associations, err := EnvironmentRecordAssociations(target, environmentName)
	if err != nil {
		return nil, err
	}
	return sortAssociations(associations), nil
}

// normalizeAssociations de-duplicates (keeping the first occurrence) and returns
// nil for an empty list so the JSON omits the field (omitempty) on the next
// write. De-duplication is a read-side safety net; the write path already
// refuses duplicates.
func normalizeAssociations(in []AssociationBinding) []AssociationBinding {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]AssociationBinding, 0, len(in))
	for _, b := range in {
		key := bindingKey(b)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	return out
}

// sortAssociations sorts by kind then subject id for a deterministic read view.
func sortAssociations(in []AssociationBinding) []AssociationBinding {
	out := make([]AssociationBinding, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SubjectKind != out[j].SubjectKind {
			return out[i].SubjectKind < out[j].SubjectKind
		}
		return out[i].SubjectID < out[j].SubjectID
	})
	return out
}

// bindingKey is the membership key: (kind, subject id).
func bindingKey(b AssociationBinding) string {
	return string(b.SubjectKind) + "\x00" + b.SubjectID
}

// refreshedAssociations reads the persisted membership list after a failed
// write, used to recover the converged binding on a concurrent divergence.
func refreshedAssociations(path string) []AssociationBinding {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var record EnvironmentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil
	}
	return normalizeAssociations(record.Associations)
}

// environmentNameForPath extracts the environment name from its state path
// (targets/<t>/.baseharbor/environments/<env>/...). It is a pure path read used
// only to re-validate a binding inside a locked mutation.
func environmentNameForPath(path string) string {
	return filepath.Base(filepath.Dir(path))
}
