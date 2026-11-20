package observability

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// ReconcileLegacySignals re-books a pre-scoping legacy observability source of
// record onto its new identity, in one atomic registry mutation. The
// ID-scoping commits (0428-168, b7cbc00f) changed the Keycloak signal ID from
// the literal "keycloak-metrics" to "keycloak:" + project; RegisterProviderSignals
// replaces every entry of the same (ID, provider) pair, so a pre-deploy legacy
// source of record would otherwise be silently dropped — gone from List and
// unremovable by its own ID. This migration re-reads that source of record (ID,
// provider, class, scope, owner, reachability, descriptor) and rewrites it
// under the new ID before the new registration, instead of dropping it.
//
// Idempotent and no-mutation: a legacy ID already absent (never written, or
// already migrated) leaves the registry untouched, and a source already carried
// under the new scoping ID is never rewritten. Removal is scoped to the owned
// providers, so a same-ID legacy registration under a foreign provider is never
// touched, and a source the caller does not own is never rewritten.
//
// LegacySource names the legacy ID plus the providers the new identity owns.
func ReconcileLegacySignals(legacy LegacySource, newID string, owned map[capability.ProviderKind]struct{}, desired []SignalSource) error {
	if err := validateLegacySource(legacy); err != nil {
		return err
	}
	if strings.TrimSpace(newID) == "" {
		return errors.New("observability legacy reconciliation requires a new identity id")
	}
	for kind := range owned {
		if kind == "" {
			return fmt.Errorf("observability legacy reconciliation id %q names an empty provider", newID)
		}
	}
	for i := range desired {
		if desired[i].ID != newID {
			return fmt.Errorf("observability legacy reconciliation id %q does not match source id %q", newID, desired[i].ID)
		}
		if _, ok := owned[desired[i].Provider]; !ok {
			return fmt.Errorf("observability legacy reconciliation source %q has a provider outside the owned set", newID)
		}
		if err := desired[i].Validate(); err != nil {
			return err
		}
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		// Re-book a legacy source of record for an owned provider (the exact
		// entry RegisterProviderSignals would drop) under the new ID, preserving
		// its identity so it stays visible and removable by its new ID.
		out := make([]SignalSource, 0, len(sources))
		for _, source := range sources {
			if _, taken := owned[source.Provider]; legacy.matches(source) && taken {
				newSource := source
				newSource.ID = newID
				out = append(out, newSource)
				continue
			}
			out = append(out, source)
		}
		out = append(out, desired...)
		return out, nil
	})
}

// validateLegacySource enforces the legacy source key: a non-empty legacy ID
// plus the set of providers the new identity owns. It is the read guard before
// any registry mutation.
func validateLegacySource(legacy LegacySource) error {
	if strings.TrimSpace(legacy.ID) == "" {
		return errors.New("observability legacy reconciliation requires a legacy id")
	}
	if len(legacy.Providers) == 0 {
		return errors.New("observability legacy reconciliation requires the new identity's owned providers")
	}
	return nil
}

// LegacySource names a legacy signal identity and the providers whose ownership
// of that identity the new identity inherits. matches reports whether an
// existing registry source is the exact legacy source of record this
// reconciliation re-books.
type LegacySource struct {
	ID        string
	Providers []capability.ProviderKind
}

func (l LegacySource) matches(source SignalSource) bool {
	if source.ID != l.ID {
		return false
	}
	for _, provider := range l.Providers {
		if source.Provider == provider {
			return true
		}
	}
	return false
}

// LegacyKeycloakMetricsID is the pre-scoping Keycloak metrics source ID the
// runtime wrote before 0428-1707 made the registration ID scoping-aware
// ("keycloak:" + project). It is the legacy ID the 0428-168 migration re-books.
const LegacyKeycloakMetricsID = "keycloak-metrics"
