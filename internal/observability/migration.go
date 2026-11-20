package observability

import (
	"errors"
	"fmt"
	"os"
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

// ScopedID derives the identity the migration books a legacy source under for
// a given project, in the canonical "<provider>:<project>:<legacy-id>"
// grammar - disjoint from any caller's "<provider>:<project>" registration
// identity, so the (ID, provider) replacement can never select it.
func (l LegacySource) ScopedID(provider capability.ProviderKind, project string) string {
	return string(provider) + ":" + strings.TrimSpace(project) + ":" + l.ID
}

// MigrateScoped re-books every legacy source of record onto its derived,
// project-scoped identity in one atomic, change-aware registry mutation (the
// shared state lock, mutateChanged), preserving every other field. It reports
// migrated=true when at least one entry was re-booked; when the legacy literal
// is already absent the registry file is not written at all.
func (l LegacySource) MigrateScoped(project string, sourceProjects ...map[string]string) (bool, error) {
	return l.MigrateScopedMany(project, sourceProjects...)
}

// MigrateScopedMany is the multi-map form: every map is consulted in order for
// an owning application's deployment project. Each match is booked under the
// CANONICAL application project (CanonicalApplicationProject), so the derived
// identity resolves through the route-2 environment derivation.
func (l LegacySource) MigrateScopedMany(project string, sourceProjects ...map[string]string) (migrated bool, err error) {
	if err := validateLegacySource(l); err != nil {
		return false, err
	}
	if strings.TrimSpace(project) == "" {
		return false, errors.New("observability legacy migration requires the replacing project")
	}
	path, err := registryPath()
	if err != nil {
		return false, err
	}
	found := false
	_, err = mutateChanged(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := make([]SignalSource, 0, len(sources))
		for _, source := range sources {
			if !l.matches(source) {
				out = append(out, source)
				continue
			}
			found = true
			move := source
			own := CanonicalApplicationProject(l.sourceProject(source, sources, project, sourceProjects...), source.OwnerApplication)
			move.ID = l.ScopedID(source.Provider, own)
			move = stampLegacyTarget(move, own)
			out = append(out, move)
		}
		return out, nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}

// sourceProject derives the project a migrated record carries: the record's
// own project when its ID is already in the scoped grammar; otherwise, for an
// application-scoped record, the deployment project of the application that
// owns it (consulting the caller's maps in order, then any scoped record that
// application's runtime wrote); otherwise the replacing deployment's project.
func (l LegacySource) sourceProject(source SignalSource, sources []SignalSource, fallback string, ownerProjects ...map[string]string) string {
	if source.Scope == capability.ScopeApplication && source.OwnerApplication != "" {
		for _, ownerProject := range ownerProjects {
			if project, ok := ownerProject[source.OwnerApplication]; ok && strings.TrimSpace(project) != "" {
				return project
			}
		}
		for _, candidate := range sources {
			if candidate.OwnerApplication != source.OwnerApplication || candidate.OwnerApplication == "" {
				continue
			}
			if project, ok := projectForApplication(candidate, source.OwnerApplication); ok {
				return project
			}
		}
	}
	return fallback
}

// projectForApplication recovers an application's deployment project from a
// registry record: the scoped ID segment, and - for records whose ID carries
// the runtime's canonical project - the application anchor inside the
// Target's runtime:// project.
func projectForApplication(source SignalSource, application string) (string, bool) {
	if project, ok := scopedProjectOf(source.ID); ok {
		return project, true
	}
	target := strings.TrimPrefix(source.Target, "runtime://")
	if idx := strings.Index(target, "/"); idx >= 0 {
		target = target[:idx]
	}
	if strings.Contains("-"+strings.ToLower(target)+"-", "-"+strings.ToLower(application)+"-") {
		return target, true
	}
	return "", false
}

// scopedProjectOf reads the project segment of a scoped ID
// "<provider>:<project>:<service>".
func scopedProjectOf(id string) (string, bool) {
	return ScopedIDProject(id)
}

// ScopedIDProject reads the project segment of a scoped ID
// "<provider>:<project>:<service>". The Keycloak registration seam uses it to
// decide, before mutating, whether a legacy record already carries its own
// anchor-bearing project (card 0428-168, r3 review fail-closed rule).
func ScopedIDProject(id string) (string, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// stampLegacyTarget repairs the environment representation of a migrated
// record WITHOUT touching how it is dialed. The route-2 environment
// derivation reads the environment as the segment after the single
// application anchor in the "bh-<target>-<app>-<env>" grammar, from BOTH the
// ID and the Target; a Target that still stands as a bare collector endpoint
// ("keycloak:9000") can never anchor, so the migration books the record's
// Target in the runtime:// grammar under the same canonical project its
// derived ID carries, keeping the service token. A record already in the
// runtime:// grammar stays as-is.
func stampLegacyTarget(source SignalSource, project string) SignalSource {
	if strings.HasPrefix(source.Target, "runtime://") {
		return source
	}
	service := source.Target
	if idx := strings.LastIndex(service, ":"); idx > 0 {
		service = service[:idx]
	}
	if service == "" {
		return source
	}
	source.Target = RuntimeTarget(project, service)
	return source
}

// canonicalApplicationProject reduces a compose project name to its canonical
// application form "bh-<target>-<app>-<env>": the environment is the LAST
// token and anything between the application token and it is a
// deployment-resource token, dropped. Names that do not carry the application
// token (shared-platform grammar, non-grammar names) return the project
// unchanged.
func CanonicalApplicationProject(project, application string) string {
	application = strings.ToLower(strings.TrimSpace(application))
	project = strings.ToLower(strings.TrimSpace(project))
	if application == "" {
		return project
	}
	tokens := strings.Split(project, "-")
	if len(tokens) < 4 || tokens[0] != "bh" {
		return project
	}
	for i := 2; i < len(tokens)-1; i++ {
		if tokens[i] == application {
			return strings.Join(tokens[:i+1], "-") + "-" + tokens[len(tokens)-1]
		}
	}
	return project
}

// RebookLegacySignals applies a LegacyReconciliation to a registry source set
// (card 0428-168): every legacy source of record the reconciliation matches is
// re-booked onto its derived identity - the owner's canonical APPLICATION
// project with the legacy signal name - so the caller's (ID, provider)
// replacement, which runs AFTER this pass in the SAME mutation, can never
// select or drop it. The record's owner and every other field stay verbatim;
// a record already migrated under a project grammar is left where the earlier
// migration placed it, and a legacy record whose owner IS the registering
// application converges onto the caller's canonical registration identity.
func RebookLegacySignals(r LegacyReconciliation, sources []SignalSource, registrationID string) []SignalSource {
	migrated := make([]SignalSource, 0, len(sources))
	for _, existing := range sources {
		if !r.Source.matches(existing) {
			migrated = append(migrated, existing)
			continue
		}
		if r.CanonicalMigrationIdentity(existing, registrationID) {
			// Do not move this legacy record onto a registration identity
			// already owned by another application. The caller will reject the
			// registration, and retaining the literal source is safer than
			// persisting a second owner under the colliding ID. Other legacy
			// records in this same pass can still be rebooked normally.
			conflict := false
			for _, candidate := range sources {
				if candidate.ID == registrationID && candidate.Provider == existing.Provider &&
					candidate.OwnerApplication != "" && candidate.OwnerApplication != existing.OwnerApplication {
					conflict = true
					break
				}
			}
			if conflict {
				migrated = append(migrated, existing)
				continue
			}
			move := existing
			move.ID = registrationID
			migrated = append(migrated, move)
			continue
		}
		move := existing
		own := CanonicalApplicationProject(r.ProjectFor(existing, sources), existing.OwnerApplication)
		move.ID = r.Source.ScopedID(existing.Provider, own)
		move = stampLegacyTarget(move, own)
		migrated = append(migrated, move)
	}
	return migrated
}

// LegacyReconciliation carries the migration context a registration needs:
// the legacy source it supersedes, the replacing deployment's project, the
// caller's application name, and the owner->project maps that name each
// owning application's canonical compose project, consulted in order.
type LegacyReconciliation struct {
	Source            LegacySource
	Project           string
	CallerApplication string
	OwnerProjects     []map[string]string
}

// CanonicalMigrationIdentity reports whether a legacy record belongs to the
// calling application itself and the caller's project carries the application
// token: the record then converges onto the caller's canonical registration
// identity instead of a derived beside-identity.
func (r *LegacyReconciliation) CanonicalMigrationIdentity(source SignalSource, registrationID string) bool {
	return source.Scope == capability.ScopeApplication && source.OwnerApplication != "" &&
		source.OwnerApplication == r.CallerApplication &&
		strings.Contains("-"+strings.ToLower(r.Project)+"-", "-"+strings.ToLower(source.OwnerApplication)+"-")
}

// ProjectFor resolves the compose project a legacy record migrates under: the
// owner->project maps first (the realization computes those from the Target's
// own deployments), then the record's own scoped ID project segment (an
// already-migrated record stays exactly where the first migration booked it),
// then any scoped record the same owner wrote, and finally the replacing
// deployment's project.
func (r *LegacyReconciliation) ProjectFor(source SignalSource, sources []SignalSource) string {
	for _, ownerProject := range r.OwnerProjects {
		if project, ok := ownerProject[source.OwnerApplication]; ok && strings.TrimSpace(project) != "" {
			return project
		}
	}
	if project, ok := scopedProjectOf(source.ID); ok {
		return project
	}
	for _, candidate := range sources {
		if candidate.OwnerApplication == "" || candidate.OwnerApplication != source.OwnerApplication {
			continue
		}
		if project, ok := projectForApplication(candidate, source.OwnerApplication); ok {
			return project
		}
	}
	return r.Project
}

// ListAll returns every source of record in the registry, unfiltered. It
// exists for callers (the legacy signal migration and its tests) that must
// map owners to their deployments across the whole registry, which the
// kind- and placement-scoped List surface cannot express.
func ListAll() ([]SignalSource, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	sources, err := read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sources, nil
}

// RegistryHasSource reports whether the registry carries (id, provider, kind)
// with the exact owning application - the predicate the legacy migration's
// tests use to prove a re-booked record kept its owner verbatim.
func RegistryHasSource(id string, provider capability.ProviderKind, kind SignalKind, ownerApplication string) (bool, error) {
	path, err := registryPath()
	if err != nil {
		return false, err
	}
	sources, err := read(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		if source.ID == id && source.Provider == provider && source.Kind == kind &&
			source.OwnerApplication == ownerApplication {
			return true, nil
		}
	}
	return false, nil
}

// RegistryHasSignal reports whether the registry still carries one exact
// signal registration. Unlike the placement-filtered listings it sees every
// provider and owner under an ID, so reconciliation and prune contracts - and
// their tests - assert retention and reclamation through it.
func RegistryHasSignal(id string, provider capability.ProviderKind, kind SignalKind) (bool, error) {
	sources, err := ListAll()
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		if source.ID == id && source.Provider == provider && source.Kind == kind {
			return true, nil
		}
	}
	return false, nil
}
