// Package environment implements the logical environment identity for
// BaseHarbor.
//
// An environment is a shared, per-Target scope that multiple applications and
// multiple execution nodes (future clusters) can belong to. Unlike an
// application name or target name, an environment name is a mutable,
// human-readable selector; it is never the durable primary key. The durable
// primary key is the environment ID: a system-generated, non-secret UUIDv4 that
// is created exactly once per (Target, environment) pair, is reused by any
// other checkout or Core that opens the same state, preserves its historical
// value on brownfield, and never changes when the environment is renamed.
//
// The ID is a property of the existing per-Target deployment state, not of a
// separate registry. This package owns only the identity value; the storage
// seam that persists it lives in internal/deployment. A separate registry file
// would introduce a second source of truth (state split, precedent 0427 — the
// rust source of several follow-ups), so no registry is created here.
package environment

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// NewID generates a fresh, valid environment identity.
func NewID() (string, error) {
	return stableid.NewUUIDv4("environment")
}

// MustNewID generates a fresh environment identity and panics on the (astronomically
// unlikely) RNG failure. Callers that must not panic should use NewID.
func MustNewID() string {
	id, err := NewID()
	if err != nil {
		panic(err)
	}
	return id
}

// Validate checks that value is a valid environment identity and returns a
// typed, actionable error otherwise.
func Validate(value string) error {
	return stableid.ValidateUUIDv4("environment", value)
}

// ValidateNonEmpty ensures value is both a non-empty string and a valid
// environment identity.
func ValidateNonEmpty(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return &stableid.ValidationError{Message: "environment id is empty"}
	}
	return Validate(value)
}

// Environment is the logical identity of one (Target, environment) pair. The
// Name is a mutable, human-readable selector; the ID is the durable key.
type Environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// New builds an Environment, allocating a fresh ID and validating the name.
func New(name string) (Environment, error) {
	id, err := NewID()
	if err != nil {
		return Environment{}, err
	}
	if err := validateName(name); err != nil {
		return Environment{}, err
	}
	return Environment{ID: id, Name: strings.TrimSpace(name)}, nil
}

// Rename returns a copy with a new Name. The ID is preserved: renaming an
// environment never changes its identity, matching the durable-identity
// guarantee in ADR 0017.
func (e Environment) Rename(name string) (Environment, error) {
	if err := validateName(name); err != nil {
		return Environment{}, err
	}
	return Environment{ID: e.ID, Name: strings.TrimSpace(name)}, nil
}

// validateSlugPattern mirrors the target-slug discipline already used across
// BaseHarbor environment names so a renamed name cannot smuggle a path or
// escape the state namespace.
var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &stableid.ValidationError{Message: "environment name is empty"}
	}
	if !namePattern.MatchString(name) {
		return &stableid.ValidationError{Message: fmt.Sprintf("environment name %q must use lowercase letters, digits, dots or hyphens and must start and end with a letter or digit", name)}
	}
	return nil
}
