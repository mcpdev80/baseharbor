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
// manifest selection + pinned release. It is fail-closed: a missing
// Perses selection or an unpinned/unsupported release always fails
// closed, as does a backend with no real signal of that release.
//
// backendSignals is the real signal inventory of the manifest (see the
// observability signal registry): a dashboard is eligible only when its
// backend actually produces the signal the dashboard queries. Passing a
// fabricated or empty inventory where a backend is expected is not
// eligibility.
func (c PersesDashboardCatalog) EligibleFor(key string, persesSelected bool, backendSignals map[DashboardBackend]bool) (DashboardEligibility, error) {
	if _, ok := c.templates[key]; !ok {
		return DashboardEligibility{
			Dashboard:     key,
			VersionPinned: c.release,
		}, fmt.Errorf("no Perses-native dashboard template for %q", key)
	}
	if !persesSelected {
		return DashboardEligibility{
			Dashboard:     key,
			Eligible:      false,
			Reason:        "perses observability surface is not selected",
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
	if !backendSignals[backend] {
		return DashboardEligibility{
			Dashboard:     key,
			Eligible:      false,
			Reason:        fmt.Sprintf("no real %s signal of %s in this manifest", backend, c.release),
			Backend:       string(backend),
			VersionPinned: c.release,
		}, nil
	}
	return DashboardEligibility{
		Dashboard:     key,
		Eligible:      true,
		Backend:       string(backend),
		VersionPinned: c.release,
	}, nil
}

// EligibleCount returns the number of eligible dashboards for the manifest
// selection + release across the whole catalog.
func (c PersesDashboardCatalog) EligibleCount(persesSelected bool, backendSignals map[DashboardBackend]bool) (int, error) {
	count := 0
	for key := range c.templates {
		elig, err := c.EligibleFor(key, persesSelected, backendSignals)
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

// FilterEligibleDashboards returns the catalog keys that are eligible for
// the manifest selection + release. Unknown keys are skipped (they are not
// part of the shipped catalog). backendSignals is the real signal
// inventory; an empty map with perses selected still yields no eligible
// dashboard, because no backend signal exists.
func FilterEligibleDashboards(catalog PersesDashboardCatalog, persesSelected bool, backendSignals map[DashboardBackend]bool) ([]string, error) {
	var eligible []string
	for key := range catalog.Templates() {
		elig, err := catalog.EligibleFor(key, persesSelected, backendSignals)
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
