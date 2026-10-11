package application

import (
	"errors"
	"fmt"
	"os"
)

// IdentitySnapshot contains claims from the existing repository/application and
// deployment owners for this repository. It is a view, never persisted state.
type IdentitySnapshot struct {
	ApplicationIDs       []string
	PreviouslyRegistered bool
}

// ResolveIdentity is read-only. An empty result means explicit initialization
// is required; it never allocates an identity during parse/inspect/plan.
func ResolveIdentity(m Manifest, snapshot IdentitySnapshot) (Manifest, error) {
	if err := m.ValidateIntent(); err != nil {
		return Manifest{}, err
	}
	ids := map[string]bool{}
	for _, id := range snapshot.ApplicationIDs {
		if err := ValidateApplicationID(id); err != nil {
			return Manifest{}, fmt.Errorf("registered application identity: %w", err)
		}
		ids[id] = true
	}
	if m.ApplicationID != "" {
		ids[m.ApplicationID] = true
	}
	if len(ids) > 1 {
		return Manifest{}, fmt.Errorf("conflicting application IDs: reconcile existing repository and deployment owners before initialization")
	}
	for id := range ids {
		m.ApplicationID = id
	}
	if m.ApplicationID == "" && snapshot.PreviouslyRegistered {
		return Manifest{}, fmt.Errorf("previously registered application identity is missing; restore the existing identity instead of allocating another")
	}
	return m, nil
}

// InitializeIdentity uses the existing canonical application Store as owner for
// an authored manifest without an ID. Call only from authorized initialization,
// after resolving real workload/source and reconciling deployment claims.
// Repository YAML is never written. Concurrent initializers converge on the
// existing Store.Create winner rather than replacing its identity.
func (s Store) InitializeIdentity(m Manifest, snapshot IdentitySnapshot) (Manifest, error) {
	authoredID := m.ApplicationID
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	items, err := s.List()
	if err != nil {
		return Manifest{}, err
	}
	for _, item := range items {
		if item.Name == m.Name {
			snapshot.ApplicationIDs = append(append([]string(nil), snapshot.ApplicationIDs...), item.ApplicationID)
			snapshot.PreviouslyRegistered = true
		}
	}
	m, err = ResolveIdentity(m, snapshot)
	if err != nil {
		return Manifest{}, err
	}
	for _, item := range items {
		if m.ApplicationID != "" && item.ApplicationID == m.ApplicationID && item.Name != m.Name {
			return Manifest{}, fmt.Errorf("application identity is already owned by %q; reconcile the existing owner before renaming", item.Name)
		}
	}
	for _, item := range items {
		if item.Name == m.Name {
			return m, nil
		}
	}
	// Existing non-store owners must be reconciled by the lifecycle adapter;
	// this method materializes only a canonical copy in the existing store.
	if m.ApplicationID == "" {
		m.ApplicationID, err = NewApplicationID()
		if err != nil {
			return Manifest{}, err
		}
	}
	if _, err = s.Create(m); err == nil {
		return m, nil
	}
	if !errors.Is(err, ErrExists) && !errors.Is(err, os.ErrExist) {
		return Manifest{}, err
	}
	winner, _, err := s.Load(m.Name)
	if err != nil {
		return Manifest{}, fmt.Errorf("concurrent initialization is incomplete; retry without creating another identity: %w", err)
	}
	// An explicit or registered ID may never silently yield to another owner.
	originalIDs := append([]string(nil), snapshot.ApplicationIDs...)
	if authoredID != "" {
		originalIDs = append(originalIDs, authoredID)
	}
	if snapshot.PreviouslyRegistered || len(originalIDs) > 0 {
		originalIDs = append(originalIDs, m.ApplicationID)
	}
	originalIDs = append(originalIDs, winner.ApplicationID)
	m.ApplicationID = winner.ApplicationID
	return ResolveIdentity(m, IdentitySnapshot{ApplicationIDs: originalIDs, PreviouslyRegistered: true})
}
