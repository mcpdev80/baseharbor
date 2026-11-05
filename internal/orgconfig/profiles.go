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

// ErrDuplicateProfile fails closed when a create/update writes a name that
// already exists under a different, non-identical definition and the caller did
// not opt into an explicit replace.
var ErrDuplicateProfile = fmt.Errorf("%w: team access profile already exists", ErrInvalidProfile)

// ErrScopeNotFound fails closed when a profile selects a Target, environment or
// operation the managed configuration does not declare. A profile can only
// reference what the authority already owns; inventing a Target or environment
// is an escalation and must be rejected before it is persisted.
var ErrScopeNotFound = fmt.Errorf("%w: profile references an undeclared target or environment", ErrInvalidProfile)

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

// Profile is a scoped team access profile: a named recommendation/assignment
// for a selected set of Targets, operations, scopes and environments. It is
// immutable once built; mutate through the Manager.
type Profile struct {
	// Name is the unique, managed identity of the profile. It must be a single
	// non-empty name token and is the key the Manager indexes on.
	Name string `json:"name" yaml:"name"`
	// Target selects the managed Target this profile grants access to. It must
	// be declared by the configuration (see Manager scope resolution).
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

// Manager owns the scoped team access profiles for one installation identity.
// A profile is a named recommendation/assignment; the Manager creates, updates
// and revokes them without ever conferring a right. The set is keyed by name;
// a name must be unique within the owner's managed set.
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
		m.profiles[strings.TrimSpace(p.Name)] = p
	}
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

	existing, ok := m.profiles[p.Name]
	if ok {
		if existing.Equal(p) {
			return existing, nil
		}
		if !replace {
			return Profile{}, ErrDuplicateProfile
		}
	}
	created := p
	m.profiles[p.Name] = created
	return created, nil
}

// Update changes an existing profile's definition. It fails closed if the name
// is unknown (ErrProfileNotFound) or the new definition is invalid. Update is
// the only way to change an owned profile: a create with a different
// definition is rejected, so an operator cannot smuggle a widening in through a
// race against Create.
func (m *Manager) Update(p Profile) (Profile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return Profile{}, ErrProfileNameEmpty
	}
	existing, ok := m.profiles[p.Name]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	p.Operations = CanonicalOperations(p.Operations...)
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	if err := m.scopeValidate(p); err != nil {
		return Profile{}, err
	}
	p.Immutable = p.ImmutableIdentity()
	if p.Immutable == existing.Immutable {
		// No visible change; return the stored identity unchanged.
		return existing, nil
	}
	updated := p
	m.profiles[p.Name] = updated
	return updated, nil
}

// Revoke removes an owned profile by name. It fails closed if the name is
// unknown. Revoking a name that resolves ambiguously (more than one managed
// profile with that name) fails closed so the caller disambiguates: the set is
// keyed by name so this can only happen across a corrupt in-memory state, which
// must never be silently resolved.
func (m *Manager) Revoke(name string) (Profile, error) {
	key := strings.TrimSpace(name)
	if key == "" {
		return Profile{}, ErrProfileNameEmpty
	}
	existing, ok := m.profiles[key]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	delete(m.profiles, key)
	return existing, nil
}

// Get returns a profile by exact name. Unknown names fail closed on
// ErrProfileNotFound.
func (m *Manager) Get(name string) (Profile, error) {
	key := strings.TrimSpace(name)
	p, ok := m.profiles[key]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	return p, nil
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
