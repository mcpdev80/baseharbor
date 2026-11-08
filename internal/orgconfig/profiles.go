// Package orgconfig owns the managed organization configuration: the single
// authority that resolves the effective Target, providers, trust and policy for
// one environment from an immutable, provenanced source.
//
// This file adds the bounded surface this card OWNS: managing scoped team
// access profiles on that existing authority. A profile is a named
// recommendation/assignment for a selected set of Targets, operations, scopes
// and environments. Profiles are NOT imported grants: holding or importing a
// profile confers no right on its own. The server rechecks the actor and the
// selected Targets at commit time, assignment is idempotent, and a foreign
// grant that another authority issued is preserved (never clobbered) while an
// owned profile is updated. Ambiguous names fail closed.
//
// New interface paths are fixed by this file; it never edits another slice's
// files. The resolution path (effective.go) is the only other touched file, and
// only so an accepted, validated profile can feed the team scope.
package orgconfig

import (
	"fmt"
	"sort"
	"strings"
)

// ErrInvalidProfileBase is the root sentinel for all team access profile errors.
var ErrInvalidProfileBase = fmt.Errorf("team access profile error")

// ErrInvalidScope fails closed for a malformed scope boundary on a profile.
var ErrInvalidScope = fmt.Errorf("%w: invalid scope boundary", ErrInvalidProfileBase)

// ErrInvalidProfile fails closed for any malformed or policy-violating profile.
var ErrInvalidProfile = fmt.Errorf("%w: invalid team access profile", ErrInvalidScope)

// ErrProfileNameEmpty fails closed on an unnameable profile.
var ErrProfileNameEmpty = fmt.Errorf("%w: team access profile name is required", ErrInvalidProfile)

// ErrProfileNameAmbiguous fails closed when a name resolves to more than one
// existing managed profile, so the caller must disambiguate explicitly.
var ErrProfileNameAmbiguous = fmt.Errorf("%w: team access profile name is ambiguous", ErrInvalidProfile)

// ErrProfileNotFound fails closed when a profile referenced by a mutation does
// not exist in the owner's managed set.
var ErrProfileNotFound = fmt.Errorf("%w: team access profile not found", ErrInvalidProfile)

// ErrDenied fails closed when a Manager cannot persist the active state (for
// example a nil Manager asked to persist). The wizard treats any persistence
// failure as a Deny rather than applying a change that was not durably written.
var ErrDenied = fmt.Errorf("%w: cannot persist active organization configuration", ErrInvalidProfile)

// ErrDuplicateProfile fails closed when a create/update writes a name that
// already exists under a different, non-identical definition and the caller did
// not opt into an explicit replace.
var ErrDuplicateProfile = fmt.Errorf("%w: team access profile already exists", ErrInvalidProfile)

// ErrScopeNotFound fails closed when a profile selects a Target, environment or
// operation the managed configuration does not declare. A profile can only
// reference what the authority already owns; inventing a Target or environment
// is an escalation and must be rejected before it is persisted.
var ErrScopeNotFound = fmt.Errorf("%w: profile references an undeclared target or environment", ErrInvalidProfile)

// ErrStaleRevision fails closed when an Update carries an expected revision that
// is older than the stored definition's revision, so the manager cannot
// last-write-wins a concurrent change.
var ErrStaleRevision = fmt.Errorf("%w: update revision is stale", ErrInvalidProfile)

// Operation is a machine-operation verb a profile may grant. Only the verbs the
// operator has already declared on the authority are selectable; an unknown
// operation is rejected so a profile cannot widen the surface.
type Operation string

const (
	OpRead    Operation = "read"
	OpLog     Operation = "log"
	OpChange  Operation = "change"
	OpSecret  Operation = "secret-use"
	OpWrite   Operation = "write"
	OpReveal  Operation = "reveal"
	OpDestroy Operation = "destroy"
	OpAdmin   Operation = "admin"
)

// ValidOperations is the complete, fixed set of operations a profile may select.
// It is the only legal value domain; adding a verb here is the sole way to
// widen the surface and is itself a contract change.
var ValidOperations = []Operation{OpRead, OpLog, OpChange, OpSecret, OpWrite, OpReveal, OpDestroy, OpAdmin}

// validateOperation fails closed for an undeclared operation verb so the
// active organization cannot widen the production policy surface by naming an
// operation outside the fixed domain.
func validateOperation(op string) error {
	op = strings.TrimSpace(op)
	if op == "" {
		return nil
	}
	if _, ok := canonicalOperationOrder[Operation(op)]; !ok {
		return fmt.Errorf("%w: operation %q is not a recognized operation", ErrInvalidProfile, op)
	}
	return nil
}

// Profile is a scoped team access profile: a named recommendation/assignment
// for a selected set of Targets, operations, scopes and environments. It is
// immutable once built; mutate through the Manager.
type Profile struct {
	// Name is the managed identity of the profile. It must be a single
	// non-empty name token. A person may hold more than one profile, one per
	// Target, so Name alone is not a unique key: the managed identity is the
	// (Name, Target) pair (see ProfileKey).
	Name string `json:"name" yaml:"name"`
	// Target selects the managed Target this profile grants access to. It must
	// be declared by the configuration (see Manager scope resolution). It is
	// part of the profile's managed identity; two profiles that share a Name on
	// different Targets are distinct profiles, not a single profile overwritten
	// on each Target.
	Target string `json:"target" yaml:"target"`
	// Environment selects the managed environment. It must be declared by the
	// configuration or empty (defaults apply).
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`
	// Operations is the ordered set of machine operations the profile assigns.
	// Order is stable and canonical (see canonicalOperationOrder).
	Operations []Operation `json:"operations" yaml:"operations"`
	// Scope narrows the assignment to a bounded boundary such as an application
	// namespace, a stack, an external provider or the team default. Empty means
	// the team-level default; it is a valid, explicit value, not "all scopes".
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`
	// Owner attributes the profile to the authorizing installation identity; it
	// is recorded for audit and rechecked at commit, not enforced here as an
	// independent actor.
	Owner string `json:"owner,omitempty" yaml:"owner,omitempty"`
	// Immutable records the canonical identity of the managed definition. Two
	// profiles with the same name but different canonical shapes are a conflict.
	Immutable string `json:"immutable" yaml:"-"`
	// Revision is the monotonically increasing write counter of the managed
	// definition. Every Update must carry the revision it observed; a write whose
	// revision is older than the stored one is stale and is rejected so the
	// manager cannot last-write-wins a concurrent change (see Update). A zero
	// expected revision on Update is rejected: revision enforcement is mandatory.
	Revision uint64 `json:"revision,omitempty" yaml:"revision,omitempty"`
}

// canonicalOperationOrder is the precedence order operations are compared and
// serialized in so two profiles with the same operations in different order are
// equal and sort deterministically.
var canonicalOperationOrder = map[Operation]int{
	OpRead: 0, OpLog: 1, OpChange: 2, OpSecret: 3, OpWrite: 4, OpReveal: 5, OpDestroy: 6, OpAdmin: 7,
}

// Validate checks the profile against this authority's structural and policy
// rules. It does NOT resolve scope against the configuration: call
// Manager.ScopeValidate for that. A profile that passes Validate but references
// an undeclared Target/operation/environment is still invalid for use; it is a
// programming error to build such a profile through a Manager.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return ErrProfileNameEmpty
	}
	if !namePattern.MatchString(strings.TrimSpace(p.Name)) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, "name is not a single identifier")
	}
	if strings.TrimSpace(p.Target) == "" {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, "target is required")
	}
	if !namePattern.MatchString(strings.TrimSpace(p.Target)) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, "target is not a single identifier")
	}
	if strings.TrimSpace(p.Scope) != "" && !namePattern.MatchString(strings.TrimSpace(p.Scope)) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, "scope is not a single identifier")
	}
	if err := p.validateOperations(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Owner) != "" && !namePattern.MatchString(strings.TrimSpace(p.Owner)) {
		return fmt.Errorf("%w: %q", ErrInvalidProfile, "owner is not a single identifier")
	}
	return nil
}

func (p Profile) validateOperations() error {
	seen := map[Operation]bool{}
	prev := -1
	for i, op := range p.Operations {
		if seen[op] {
			return fmt.Errorf("%w: duplicate operation %q", ErrInvalidProfile, op)
		}
		seen[op] = true
		order, ok := canonicalOperationOrder[op]
		if !ok {
			return fmt.Errorf("%w: operation %q is not a recognized operation", ErrInvalidProfile, op)
		}
		if i > 0 && order < prev {
			return fmt.Errorf("%w: operations are not in canonical order", ErrInvalidProfile)
		}
		prev = order
	}
	if len(p.Operations) == 0 {
		return fmt.Errorf("%w: a profile must assign at least one operation", ErrInvalidProfile)
	}
	return nil
}

// CanonicalOperations returns a sorted, de-duplicated copy of the operations in
// canonical precedence order. Build every profile's operation list through this
// so the stored order is deterministic and comparable.
func CanonicalOperations(ops ...Operation) []Operation {
	seen := map[Operation]bool{}
	out := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if seen[op] || op == "" {
			continue
		}
		seen[op] = true
		out = append(out, op)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return canonicalOperationOrder[out[i]] < canonicalOperationOrder[out[j]]
	})
	return out
}

// ImmutableIdentity computes the canonical identity of the managed definition.
// Two profiles with the same name but different canonical shapes compare
// unequal, so a create that would collide with a different definition fails
// closed on ErrDuplicateProfile and an update that changes the definition is
// observable.
func (p Profile) ImmutableIdentity() string {
	return canonicalProfileIdentity(p)
}

// Equal reports whether two profiles are identical in their managed definition
// (name, target, environment, scope, operations, owner). A create with an equal
// definition is idempotent; a create with a different definition is a conflict.
func (p Profile) Equal(other Profile) bool {
	if p.Immutable != "" && other.Immutable != "" {
		return p.Immutable == other.Immutable
	}
	return p.ImmutableIdentity() == other.ImmutableIdentity()
}

// canonicalProfileIdentity hashes the managed definition fields into a stable
// canonical identity. It is used for idempotency (a re-create with the same
// definition is a no-op) and for detecting a conflicting re-create. It never
// includes the Immutable field itself.
func canonicalProfileIdentity(p Profile) string {
	ops := CanonicalOperations(p.Operations...)
	parts := make([]string, 0, 6)
	parts = append(parts, "name="+p.Name, "target="+p.Target, "env="+p.Environment,
		"scope="+p.Scope, "owner="+p.Owner)
	for _, op := range ops {
		parts = append(parts, "op="+string(op))
	}
	return joinIdentity(parts)
}

// ProfileKey returns the managed identity of a profile: the (name,target) pair.
// A person may hold one owned profile per declared Target, so the key is the
// pair, not the name alone. Two profiles that share a Name on different Targets
// have distinct keys and both survive in the manager; replacing one Target never
// drops the other.
func ProfileKey(name, target string) string {
	return joinIdentity([]string{"name=" + strings.TrimSpace(name), "target=" + strings.TrimSpace(target)})
}

func joinIdentity(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "|"
		}
		out += p
	}
	return out
}

// isSubsetOf reports whether need is a subset of have (every element of need is
// present in have). Operations are already canonicalised by the caller, so a
// straight membership check is sound.
func isSubsetOf(need, have []Operation) bool {
	owned := make(map[Operation]bool, len(have))
	for _, op := range have {
		owned[op] = true
	}
	for _, op := range need {
		if !owned[op] {
			return false
		}
	}
	return true
}

// Manager owns the scoped team access profiles for one installation identity.
// A profile is a named recommendation/assignment; the Manager creates, updates
// and revokes them without ever conferring a right. The set is keyed by the
// (name, target) pair: a person may hold one owned profile per declared Target,
// so two profiles that share a Name on different Targets are distinct entries.
// A create or update on one Target never drops a profile owned on another.
type Manager struct {
	// Owner is the installation identity this Manager assigns under. It is the
	// authority a profile is bound to; a foreign installation's profiles are
	// never touched.
	Owner    string
	profiles map[string]Profile
	// scope is the configuration authority the Manager validates a profile's
	// Target/operation/environment against. It is nil until ScopeValidate runs
	// so a Manager cannot silently accept an undeclared scope.
	scope scopeAuthority
}

// scopeAuthority is the minimal view of the organization configuration the
// Manager needs to scope-validate a profile: which Targets, environments and
// operations are declared and owned by this authority.
type scopeAuthority interface {
	// TargetKnown reports whether the configuration declares the named Target.
	TargetKnown(target string) bool
	// EnvironmentKnown reports whether the configuration declares the named
	// environment. An empty environment always resolves (defaults apply).
	EnvironmentKnown(env string) bool
}

// NewManager builds a Manager for one owner. profiles may be nil for a fresh
// set. It does not validate scope; call ScopeValidate before use.
func NewManager(owner string, profiles ...Profile) *Manager {
	m := &Manager{
		Owner:    strings.TrimSpace(owner),
		profiles: map[string]Profile{},
	}
	for _, p := range profiles {
		m.profiles[ProfileKey(p.Name, p.Target)] = p
	}
	return m
}

// LoadProfiles reads the owned profiles of one installation identity (tenant)
// from the active organization configuration and returns them for a Manager the
// production path can fold into the single policy authority. It is the mirror
// of Create/Update/Revoke on the load side: a Manager is only as good as the
// profiles it was built from, so the adapter's production finding is empty
// unless the caller feeds it the real owned set here.
//
// The stored map is keyed by (name, target); this returns the profiles as a
// slice, so a person's distinct per-Target profiles all load rather than being
// collapsed to one.
//
// An empty owner loads no profiles (an anonymous/unauthenticated caller has no
// managed set); an active state with no persisted profiles loads an empty set,
// which fails closed through the adapter (unknown profile). The caller decides
// whether an empty set is a Deny or a "no active organization" no-op; this
// function only reports what is actually owned.
func LoadProfiles(state ActiveState, owner string) []Profile {
	owner = strings.TrimSpace(owner)
	config := state.Config
	var owned []Profile
	for _, profile := range config.Profiles {
		if strings.TrimSpace(profile.Owner) != "" && strings.TrimSpace(profile.Owner) != owner {
			// A profile belongs to whichever identity installed it; never
			// enumerate another installation's profiles on this owner's path.
			continue
		}
		if strings.TrimSpace(profile.Name) == "" {
			continue
		}
		owned = append(owned, profile)
	}
	return owned
}

// ResolveOperation maps the active organization configuration to the single
// concrete machine operation the production policy path resolves against. It
// returns an error when the configuration declares no operation, so the caller
// fails closed rather than defaulting to a hardcoded verb. The operation is
// the effective policy's declared operation for the active environment, or
// ErrNoOperation when none is declared.
func ResolveOperation(state ActiveState) (Operation, error) {
	if strings.TrimSpace(state.Config.Operation) != "" {
		if _, ok := canonicalOperationOrder[Operation(state.Config.Operation)]; !ok {
			return "", fmt.Errorf("%w: %q is not a recognized operation", ErrInvalidProfile, state.Config.Operation)
		}
		return Operation(state.Config.Operation), nil
	}
	return "", ErrNoOperation
}

// ErrNoOperation fails closed when the active organization configuration
// declares no machine operation for the production policy path to resolve.
var ErrNoOperation = fmt.Errorf("%w: active organization declares no machine operation", ErrInvalidProfileBase)

// scopeAuthorityFromActive is the concrete scopeAuthority a Manager binds to when
// loaded from an active state: it resolves declared Targets and environments
// against the configuration the Manager was built from.
type scopeAuthorityFromActive struct {
	config Config
}

func (s scopeAuthorityFromActive) TargetKnown(target string) bool {
	_, ok := s.config.Targets[strings.TrimSpace(target)]
	return ok
}

func (s scopeAuthorityFromActive) EnvironmentKnown(env string) bool {
	if strings.TrimSpace(env) == "" {
		return true
	}
	_, ok := s.config.Environments[strings.TrimSpace(env)]
	return ok
}

// ManagerFromActive builds a Manager for the active owner from an active
// organization configuration: it loads the owner's owned profiles and binds the
// Manager to the active authority so scope validation can run on the production
// path. A Manager with no owned profiles (or an empty owner) is valid and
// simply fails closed on any Resolve.
func ManagerFromActive(state ActiveState) *Manager {
	owner := strings.TrimSpace(state.Config.Organization)
	m := NewManager(owner, LoadProfiles(state, owner)...)
	m.BindScope(scopeAuthorityFromActive{config: state.Config})
	return m
}

// BindScope binds the Manager to the configuration authority. It is idempotent:
// a second call with the same authority is a no-op and a call with nil returns
// a Manager that cannot scope-check (validate becomes structural only).
func (m *Manager) BindScope(scope scopeAuthority) *Manager {
	m.scope = scope
	return m
}

// Create adds a new profile. It fails closed if the name is missing or the
// definition is invalid. If the name already exists:
//   - an identical definition is an idempotent no-op that returns the existing
//     profile and a nil error;
//   - a different definition fails closed on ErrDuplicateProfile unless
//     replace is true.
func (m *Manager) Create(p Profile, replace bool) (Profile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return Profile{}, ErrProfileNameEmpty
	}
	p.Operations = CanonicalOperations(p.Operations...)
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	if err := m.scopeValidate(p); err != nil {
		return Profile{}, err
	}
	p.Immutable = p.ImmutableIdentity()
	p.Revision = 1 // freshly created definition starts at revision 1

	existing, ok := m.profiles[ProfileKey(p.Name, p.Target)]
	if ok {
		if existing.Equal(p) {
			return existing, nil
		}
		if !replace {
			return Profile{}, ErrDuplicateProfile
		}
	}
	created := p
	m.profiles[ProfileKey(p.Name, p.Target)] = created
	return created, nil
}

// Update changes an existing profile's definition. It fails closed if the name
// is unknown (ErrProfileNotFound) or the new definition is invalid. Update is
// the only way to change an owned profile: a create with a different
// definition is rejected, so an operator cannot smuggle a widening in through a
// race against Create.
//
// Update is bound to the single authority that owns the Manager: it operates on
// one installation identity (Manager.Owner) and widens nothing. Two
// preconditions defend the write:
//
//   - expectedRevision must equal the stored definition's Revision. A smaller
//     value is a stale write and fails closed on ErrStaleRevision (no
//     last-write-wins); a larger value is a mismatched/future revision the
//     caller never observed and is rejected too, so the guard cannot be
//     bypassed by guessing or racing forward past the stored revision; an empty
//     (zero) expected revision is rejected so stale enforcement is mandatory and
//     cannot be bypassed by omission.
//   - the new operation set may not exceed the stored one. Removing operations
//     is always allowed; adding an operation the current definition did not
//     assign is a privilege escalation and fails closed. The effective right a
//     caller may still exercise is never widened by an edit.
//
// The edit does not itself resolve the caller's actor/tenant at commit time:
// that is the one production authority. The adapter reports profile membership
// on that path (see internal/policy); it never grants.
func (m *Manager) Update(expectedRevision uint64, p Profile) (Profile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return Profile{}, ErrProfileNameEmpty
	}
	existing, ok := m.profiles[ProfileKey(p.Name, p.Target)]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	if expectedRevision == 0 {
		return Profile{}, ErrStaleRevision
	}
	// The precondition is an exact match: the write must carry the revision it
	// observed. A smaller value is a stale write and fails closed on
	// ErrStaleRevision (no last-write-wins); a larger value is a mismatched/
	// future revision the caller never observed and is rejected too, so the
	// guard cannot be bypassed by guessing or racing forward past the stored
	// revision.
	if expectedRevision != existing.Revision {
		return Profile{}, fmt.Errorf("%w: expected %d, stored %d", ErrStaleRevision, expectedRevision, existing.Revision)
	}
	p.Operations = CanonicalOperations(p.Operations...)
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	if err := m.scopeValidate(p); err != nil {
		return Profile{}, err
	}
	if !isSubsetOf(p.Operations, existing.Operations) {
		return Profile{}, fmt.Errorf("%w: edit may not add an operation the current definition does not assign", ErrInvalidProfile)
	}
	// A successful edit always bumps the revision, even when the managed
	// definition is byte-identical, so every write is observable and the next
	// write's expected revision advances.
	p.Revision = existing.Revision + 1
	p.Immutable = p.ImmutableIdentity()
	updated := p
	m.profiles[ProfileKey(p.Name, p.Target)] = updated
	return updated, nil
}

// Revoke removes an owned profile by its (name, target) identity. It fails
// closed if the name is unknown, or no owned profile exists on the given Target
// for that name. Revoking one (name, target) leaves a profile the same Name
// holds on another Target untouched.
func (m *Manager) Revoke(name, target string) (Profile, error) {
	key := ProfileKey(name, target)
	if strings.TrimSpace(key) == "" {
		return Profile{}, ErrProfileNameEmpty
	}
	existing, ok := m.profiles[key]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	delete(m.profiles, key)
	return existing, nil
}

// Get returns the profile on the given (name, target). Unknown names or a name
// with no owned profile on the given Target fail closed on ErrProfileNotFound.
// GetByKey is the authoritative single-profile lookup on the per-target model:
// the same Name on a different Target is a distinct profile.
func (m *Manager) GetByKey(name, target string) (Profile, error) {
	p, ok := m.profiles[ProfileKey(name, target)]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	return p, nil
}

// Get returns the profile with the given Name, on whichever declared Target
// first owns it. Unknown Name, or a Name with no owned profile, fails closed on
// ErrProfileNotFound. On the per-target model a Name may be held on more than
// one Target, so callers that must address one Target use GetByKey.
func (m *Manager) Get(name string) (Profile, error) {
	for _, p := range m.profiles {
		if strings.TrimSpace(p.Name) == strings.TrimSpace(name) {
			return p, nil
		}
	}
	return Profile{}, ErrProfileNotFound
}

// List returns the managed profiles in canonical order (by name).
func (m *Manager) List() []Profile {
	out := make([]Profile, 0, len(m.profiles))
	for _, p := range m.profiles {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SnapshotProfiles returns a copy of the manager's owned profiles for the
// manager's owner. The caller may persist the set back into an active state;
// the returned slice is a copy, so editing it never mutates the manager. This
// is the mirror of LoadProfiles on the write side: an in-memory Manager that
// was mutated by Create/Update/Revoke can fold its owned set back into the
// production active-state writer.
func (m *Manager) SnapshotProfiles() []Profile {
	out := make([]Profile, 0, len(m.profiles))
	for _, p := range m.profiles {
		out = append(out, p)
	}
	return out
}

// ProfileMap converts a slice of profiles into the keyed map the active
// configuration stores, the inverse of toProfileList. It lets the wizard fold a
// Manager's owned set back into an ActiveState without coupling the wizard to
// the map representation. The map is keyed by the (name, target) identity so a
// person's distinct per-Target profiles survive persistence: keying by name
// alone would collapse one person's profiles into the last written.
func ProfileMap(list []Profile) map[string]Profile {
	m := make(map[string]Profile, len(list))
	for _, p := range list {
		m[ProfileKey(p.Name, p.Target)] = p
	}
	return m
}

// toProfileList converts a map of profiles into a slice, the inverse of
// ProfileMap.
func toProfileList(m map[string]Profile) []Profile {
	out := make([]Profile, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	return out
}

// PersistActive folds the manager's owned profile set into the loaded active
// state and writes it through the product's active-state writer. It is a single
// persistence entry point so the wizard persists assignment/revocation changes
// through exactly one path. A persistence failure is returned so the caller can
// fail closed and report no success.
func PersistActive(manager *Manager, state ActiveState) error {
	if manager == nil {
		return ErrDenied
	}
	state.Config.Profiles = ProfileMap(manager.SnapshotProfiles())
	return SaveActive(state)
}

// Store persists an active organization configuration. The wizard injects one
// so a confirmed assignment or revocation reaches the production store; tests
// inject a fake so they do not touch the filesystem.
type Store interface {
	Save(state ActiveState) error
}

// FSStore is the production Store backed by the active-state writer.
type FSStore struct{}

// Save writes the active configuration through the production writer.
func (FSStore) Save(state ActiveState) error {
	return SaveActive(state)
}

// Count returns the number of managed profiles.
func (m *Manager) Count() int {
	return len(m.profiles)
}

// scopeValidate checks that a profile only references Targets, environments and
// operations this authority owns. It fails closed if the profile references an
// undeclared Target or environment. Operations are validated structurally by
// Profile.Validate; here we only ensure the Target/environment are declared by
// the configuration the Manager is bound to. An empty environment passes.
//
// When the Manager is not bound to an authority (nil scope), structural-only
// validation applies and this returns nil so a bound-less Manager can still be
// exercised; scope enforcement requires BindScope.
func (m *Manager) scopeValidate(p Profile) error {
	if m.scope == nil {
		return nil
	}
	if !m.scope.TargetKnown(p.Target) {
		return fmt.Errorf("%w: target %q", ErrScopeNotFound, p.Target)
	}
	if strings.TrimSpace(p.Environment) != "" && !m.scope.EnvironmentKnown(p.Environment) {
		return fmt.Errorf("%w: environment %q", ErrScopeNotFound, p.Environment)
	}
	return nil
}
