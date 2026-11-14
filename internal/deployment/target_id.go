package deployment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// TargetIdentityKind is the stableid label used for the stable Target UID.
const TargetIdentityKind = "target"

// ErrTargetUIDCollision is returned when a name-to-UID migration would bind two
// distinct Target names to one UID, or a caller tries to assign an existing
// UID to another name. The identity model keeps the UID canonical and the name
// addressable, so a collision fails closed rather than silently re-binding.
var ErrTargetUIDCollision = fmt.Errorf("target uid collision")

// ValidateTargetUID enforces that a Target UID is a lowercase UUIDv4 and never
// conflates the three distinct identity roles: the stable Target UID is a
// Core-side identity that is separate from the TenantID (Core installation
// ownership path), from any workstation alias, and from a deployment id. An
// empty UID is allowed for Targets created before stable identity existed; a
// present UID must validate.
func ValidateTargetUID(uid string) error {
	return stableid.ValidateUUIDv4(TargetIdentityKind, uid)
}

// NewTargetUID generates a fresh, unique stable Target UID.
func NewTargetUID() (string, error) {
	return stableid.NewUUIDv4(TargetIdentityKind)
}

// MigrateTargetUID resolves the stable Target UID for one Target name, creating
// and returning a new UID only when the definition does not yet carry one. The
// migration is idempotent: a second call with the same definition returns the
// already-stored UID and reports Created=false. It never moves data on disk;
// the Target's state directories remain keyed by the name — only the resolution
// gains the UID.
//
// A UID supplied on an existing definition is validated but never compared
// across definitions here; cross-definition uniqueness is the caller's job
// (see ResolveTargetUIDs), because a create migration must reject a collision
// against the live configuration, not the definition under mutation.
func MigrateTargetUID(def TargetDefinition) (uid string, created bool, err error) {
	uid = strings.TrimSpace(def.TargetUID)
	if uid == "" {
		uid, err = NewTargetUID()
		if err != nil {
			return "", false, err
		}
		return uid, true, nil
	}
	if err := ValidateTargetUID(uid); err != nil {
		return "", false, err
	}
	return uid, false, nil
}

// ApplyTargetUID mutates def in place with the resolved UID. It is a pure
// helper for create paths that must persist the UID on the written definition.
func ApplyTargetUID(def *TargetDefinition, uid string) {
	def.TargetUID = strings.ToLower(strings.TrimSpace(uid))
}

// ResolveTargetUIDs returns the stable UID for every configured Target, keyed by
// name. It fails closed when two or more distinct names would resolve to the
// same UID, because that poisons the name→UID resolution for both. This is the
// write-seam uniqueness check a create migration must run against the full,
// post-mutation configuration before persisting.
func ResolveTargetUIDs(cfg Config) (map[string]string, error) {
	uids := make(map[string]string, len(cfg.Targets))
	for name := range cfg.Targets {
		uid, _, err := MigrateTargetUID(cfg.Targets[name])
		if err != nil {
			return nil, err
		}
		uids[name] = uid
	}
	if err := validateTargetUIDUniqueness(cfg); err != nil {
		return nil, err
	}
	return uids, nil
}

// CheckTargetUIDCollision reports whether assigning uid to name would collide
// with an already-resolved UID of a DIFFERENT name in the given map. The map is
// the post-mutation configuration (the name being created already carries uid).
// An empty uid (unmigrated) is never a collision. This is the fail-closed
// guard for a create that supplies an explicit UID.
func CheckTargetUIDCollision(uids map[string]string, name, uid string) bool {
	uid = strings.ToLower(strings.TrimSpace(uid))
	if uid == "" {
		return false
	}
	for other, existing := range uids {
		if other == name {
			continue
		}
		if strings.ToLower(strings.TrimSpace(existing)) == uid {
			return true
		}
	}
	return false
}

// validateTargetUIDUniqueness enforces that no two distinct configured Targets
// resolve to one UID. It is the read-and-write seam check: any Config.Validate
// run rejects a poisoned configuration so a collision can never be persisted or
// loaded. Empty UIDs are ignored (pre-stable-identity definitions migrate later).
func validateTargetUIDUniqueness(cfg Config) error {
	seen := make(map[string]string, len(cfg.Targets))
	for name, def := range cfg.Targets {
		uid := strings.TrimSpace(def.TargetUID)
		if uid == "" {
			continue
		}
		uid = strings.ToLower(uid)
		if existing, ok := seen[uid]; ok {
			return fmt.Errorf("target uid collision: %q and %q resolve to the same stable target uid %q", existing, name, uid)
		}
		seen[uid] = name
	}
	return nil
}

// UniqueTargetUIDs is a small helper used by tests: it sorts the name→UID pairs
// into a stable, human-readable report string so collision tests can assert on
// a deterministic value.
func UniqueTargetUIDs(uids map[string]string) (string, error) {
	names := make([]string, 0, len(uids))
	for name := range uids {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+uids[name])
	}
	return strings.Join(parts, ", "), nil
}
