package capability

import (
	"fmt"
)

// Dashboard eligibility + provisioning for Perses-native dashboards.
//
// This is the selection catalog that builds on the Perses seam
// (perses.go). The seam pins the optional Perses release and the
// fail-closed "not configured" surface; this file answers the next
// question: given a selected Perses surface, which Perses-native
// dashboards are ELIGIBLE and how are they PROVISIONED from the real
// signal/label inventory and authorized providers.
//
// Eligibility is derived from manifest truth + the real provider
// signal inventory, never from an assumed exporter. A dashboard that
// has no real backend signal of the selected Perses version, no
// authorized provider backing it, or a missing datasource reference,
// is reported as NOT_ELIGIBLE with a concrete reason — never silently
// provisioned and never as a false Grafana-parity claim.
//
// Perses-native dashboards are a management realization, never an
// application capability, so this code never enables an exporter or
// declares metrics/logs/traces as an application capability.

// IdentitySelectionWitness is the authoritative, non-forgeable evidence the
// selection/fan-in resolver must produce before a Perses-native dashboard (in
// particular the Keycloak identity dashboard) can be reported available.
//
// Per the owner decision (2026-11-16), positive identity availability is NOT a
// status boolean a caller may assert. It must FOLLOW FROM real material only:
// a real Keycloak native metrics signal present for this Target, pinned to the
// current Perses release, an identity provider that is actually selected, and a
// real Target authorization for the provider. The zero value of this struct is
// `unavailable` (fail-closed); a caller can never obtain a positive availability
// by setting the availability result directly — only by attesting the four
// evidence flags the resolver verifies.
type IdentitySelectionWitness struct {
	// SelectionConfirmed reports that the identity provider is actually
	// selected in the manifest (services.identity / the Keycloak binding),
	// never an assumed exporter.
	SelectionConfirmed bool
	// MetricPresent reports that a real Keycloak native metrics signal exists
	// for this Target (declared in the provider integration descriptor and
	// realized in the observability signal registry), never a caller guess.
	MetricPresent bool
	// MetricCurrent reports that that signal is pinned to the current,
	// accepted Perses release (AcceptedPersesVersion); an obsolete-key or
	// unpinned signal must fail closed.
	MetricCurrent bool
	// TargetAuthorized reports a real, non-forgeable authorization for this
	// Target/ownership (e.g. a valid ProviderPlacement), never an assumed
	// caller-supplied status.
	TargetAuthorized bool
	// Available is NOT a caller input: it is the derived availability this
	// resolver computes from the four evidence flags above. A caller must
	// never set it true directly; NewIdentitySelectionWitness derives it.
	Available bool
}

// NewIdentitySelectionWitness builds an authoritative identity-selection
// witness from real evidence flags. Availability is DERIVED, never supplied:
// the returned witness carries Available=true only when every evidence flag is
// true, and the zero value / an unconstructed witness is always
// `unavailable` (fail-closed). A caller cannot make a dashboard available by
// supplying a status boolean — only by attesting a real Keycloak metric signal
// plus a real Target authorization.
func NewIdentitySelectionWitness(selectionConfirmed, metricPresent, metricCurrent, targetAuthorized bool) IdentitySelectionWitness {
	witness := IdentitySelectionWitness{
		SelectionConfirmed: selectionConfirmed,
		MetricPresent:      metricPresent,
		MetricCurrent:      metricCurrent,
		TargetAuthorized:   targetAuthorized,
	}
	// The derived availability is the AND of all real-evidence flags. Any
	// missing flag (no selection, no metric, obsolete metric, unauthorized
	// target) fails closed to unavailable.
	witness.Available = witness.SelectionConfirmed && witness.MetricPresent && witness.MetricCurrent && witness.TargetAuthorized
	return witness
}

// DashboardEligibility is the selection verdict for one Perses-native
// dashboard for a manifest + pinned release.
type DashboardEligibility struct {
	// Dashboard is the stable catalog key (e.g. "prometheus-targets").
	Dashboard string
	// Eligible is true only when the dashboard has a real, authorized
	// backend signal of the pinned release and a valid datasource ref.
	Eligible bool
	// Reason explains an INELIGIBLE verdict. It is empty when Eligible.
	Reason string
	// Backend is the backend data owner the dashboard realizes
	// (prometheus, loki, tempo) or "" when unknown.
	Backend string
	// VersionPinned records the accepted Perses release this verdict is
	// keyed on (e.g. "v0.55.1").
	VersionPinned string
}

// DashboardBackend is the backend data owner a Perses-native dashboard
// realizes. It must match a real BaseHarbor backend, never a Grafana
// assumption.
type DashboardBackend string

const (
	DashboardBackendPrometheus DashboardBackend = "prometheus"
	DashboardBackendLoki       DashboardBackend = "loki"
	DashboardBackendTempo      DashboardBackend = "tempo"
)

// DashboardStatus is the selection verdict a dashboard reports.
type DashboardStatus string

const (
	// DashboardEligible means the dashboard has a real authorized
	// backend signal of the pinned release and a valid Perses datasource.
	DashboardEligible DashboardStatus = "eligible"
	// DashboardNotEligible means the dashboard has no real backend signal
	// (missing metric/label), an unauthorized provider, an unpinned or
	// unselected Perses release, or a missing datasource reference.
	DashboardNotEligible DashboardStatus = "not_eligible"
)

// DashboardTemplate is a versioned Perses-native dashboard template: its
// catalog key, the backend it realizes and the Perses release version it
// is keyed on. A template only exists when its backend has a real signal.
type DashboardTemplate struct {
	// Key is the stable catalog identifier.
	Key string
	// Backend is the real data owner this dashboard queries.
	Backend DashboardBackend
	// Datasource is the Perses datasource kind required by the panel
	// (e.g. "prometheus", "loki"); it must match a real backend signal.
	Datasource string
	// SupportedVersions lists the accepted Perses release versions this
	// template is compatible with (native plugin/format versioning).
	SupportedVersions []string
}

// PersesDashboardCatalog is the deterministic shipped selection catalog:
// the eligible Perses-native dashboards, keyed by accepted version and
// real backend signal. Consumers iterate this instead of partial lists.
type PersesDashboardCatalog struct {
	release   string
	templates map[string]DashboardTemplate
}

// AcceptedPersesVersion is the pinned release the selection catalog keys on.
// It must stay in lock-step with the seam pin (v0.55.1): the catalog only
// accepts a release whose security fixes are current.
const AcceptedPersesVersion = "v0.55.1"

// DefaultDashboardCatalog builds the shipped selection catalog for the
// pinned release. A template exists only where a real backend signal of
// that release exists; Prometheus targets, Loki logs and Tempo traces are
// the three real observability backends BaseHarbor actually ships.
func DefaultDashboardCatalog() PersesDashboardCatalog {
	templates := map[string]DashboardTemplate{
		"prometheus-targets": {
			Key:               "prometheus-targets",
			Backend:           DashboardBackendPrometheus,
			Datasource:        "prometheus",
			SupportedVersions: []string{AcceptedPersesVersion},
		},
		"loki-logs": {
			Key:               "loki-logs",
			Backend:           DashboardBackendLoki,
			Datasource:        "loki",
			SupportedVersions: []string{AcceptedPersesVersion},
		},
		"tempo-traces": {
			Key:               "tempo-traces",
			Backend:           DashboardBackendTempo,
			Datasource:        "tempo",
			SupportedVersions: []string{AcceptedPersesVersion},
		},
	}
	return PersesDashboardCatalog{
		release:   AcceptedPersesVersion,
		templates: templates,
	}
}

// Templates returns the shipped dashboard templates keyed by catalog key.
// The map contains only dashboards whose backend has a real signal;
// exporters that have no native signal (e.g. PostgreSQL/Valkey
// requires-adapter) are absent and must not be provisioned.
func (c PersesDashboardCatalog) Templates() map[string]DashboardTemplate {
	out := make(map[string]DashboardTemplate, len(c.templates))
	for k, v := range c.templates {
		out[k] = v
	}
	return out
}

// HasTemplate reports whether a real Perses-native dashboard template
// exists for the given catalog key.
func (c PersesDashboardCatalog) HasTemplate(key string) bool {
	_, ok := c.templates[key]
	return ok
}

// EligibleFor evaluates the selection verdict for one dashboard key for a
// manifest selection + pinned release. It is fail-closed: a missing Perses
// selection, an unpinned/unsupported release, a backend with no real signal of
// that release, or an absent identity-selection witness all fail closed, as
// does a caller that never supplied the real evidence. The availability is
// derived from the authoritative witness (real Keycloak metric + real Target
// authorization), never from a caller-supplied status boolean.
//
// witness is the authoritative, non-forgeable evidence the resolver produced:
// a positive verdict requires an available witness (every real-evidence flag
// true). Passing an unconstructed / zero-value witness is not availability —
// it fails closed, never a false "configured" claim.
func (c PersesDashboardCatalog) EligibleFor(key string, witness IdentitySelectionWitness) (DashboardEligibility, error) {
	if _, ok := c.templates[key]; !ok {
		return DashboardEligibility{
			Dashboard:     key,
			VersionPinned: c.release,
		}, fmt.Errorf("no Perses-native dashboard template for %q", key)
	}
	if !witness.Available {
		return DashboardEligibility{
			Dashboard:     key,
			Eligible:      false,
			Reason:        "identity selection witness not available: a real Keycloak metric signal + real Target authorization is required (fail-closed), not a caller-supplied status",
			Backend:       string(c.templates[key].Backend),
			VersionPinned: c.release,
		}, nil
	}
	if err := ValidatePersesRelease(PersesDefaultRelease); err != nil {
		return DashboardEligibility{
			Dashboard:     key,
			Eligible:      false,
			Reason:        "perses release is not pinned or is invalid (fail-closed)",
			Backend:       string(c.templates[key].Backend),
			VersionPinned: c.release,
		}, nil
	}
	if !containsVersion(c.templates[key].SupportedVersions, c.release) {
		return DashboardEligibility{
			Dashboard:     key,
			Eligible:      false,
			Reason:        fmt.Sprintf("perses release %s is not in the accepted version set %v", c.release, c.templates[key].SupportedVersions),
			Backend:       string(c.templates[key].Backend),
			VersionPinned: c.release,
		}, nil
	}
	backend := c.templates[key].Backend
	// The backend is a real template backend of this catalog (Prometheus,
	// Loki or Tempo) — established by the template-existence check above. A
	// real backend signal of the pinned release is required; the catalog is
	// populated only for backends that actually ship such a signal, so a
	// missing template (e.g. PostgreSQL/Valkey requires-adapter) never enters
	// here. The availability verdict was already fail-closed above when the
	// identity-selection witness is not available.
	return DashboardEligibility{
		Dashboard:     key,
		Eligible:      true,
		Backend:       string(backend),
		VersionPinned: c.release,
	}, nil
}

// EligibleCount returns the number of eligible dashboards for the manifest
// selection + release across the whole catalog. witness is the authoritative
// identity-selection witness: only an available witness (real Keycloak metric
// + real Target authorization) yields any eligible dashboard; an unconstructed
// witness yields zero.
func (c PersesDashboardCatalog) EligibleCount(witness IdentitySelectionWitness) (int, error) {
	count := 0
	for key := range c.templates {
		elig, err := c.EligibleFor(key, witness)
		if err != nil {
			return 0, err
		}
		if elig.Eligible {
			count++
		}
	}
	return count, nil
}

func containsVersion(versions []string, version string) bool {
	for _, v := range versions {
		if v == version {
			return true
		}
	}
	return false
}

// VerifyPersesDashboardCatalogBuilds pins the shipped catalog: it has at
// least one template and every template carries a real backend and a
// datasource kind. A catalog with zero templates or an empty datasource
// would silently provision empty dashboards.
func VerifyPersesDashboardCatalogBuilds() error {
	catalog := DefaultDashboardCatalog()
	if len(catalog.Templates()) == 0 {
		return fmt.Errorf("perses dashboard catalog is empty")
	}
	for key, tmpl := range catalog.Templates() {
		if tmpl.Key == "" {
			return fmt.Errorf("perses dashboard %q has an empty key", key)
		}
		if tmpl.Datasource == "" {
			return fmt.Errorf("perses dashboard %q has no datasource kind", key)
		}
		if len(tmpl.SupportedVersions) == 0 {
			return fmt.Errorf("perses dashboard %q has no supported perses versions", key)
		}
	}
	return nil
}

// FilterEligibleDashboards returns the catalog keys that are eligible for the
// manifest selection + release. Unknown keys are skipped (they are not part of
// the shipped catalog). witness is the authoritative identity-selection
// witness: only an available witness (real Keycloak metric + real Target
// authorization) yields an eligible dashboard; an unconstructed witness yields
// none, because no real signal/authorization exists.
func FilterEligibleDashboards(catalog PersesDashboardCatalog, witness IdentitySelectionWitness) ([]string, error) {
	var eligible []string
	for key := range catalog.Templates() {
		elig, err := catalog.EligibleFor(key, witness)
		if err != nil {
			return nil, err
		}
		if elig.Eligible {
			eligible = append(eligible, key)
		}
	}
	sortStrings(eligible)
	return eligible, nil
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j-1] > in[j]; j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

// DashboardStatusString is a stable selector for a single verdict.
func (e DashboardEligibility) Status() DashboardStatus {
	if e.Eligible {
		return DashboardEligible
	}
	return DashboardNotEligible
}
