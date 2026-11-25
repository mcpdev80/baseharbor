package metrics

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// PersesBackendAnswersStatusNow maps the resolved manifest through the real
// per-target Perses health probe to the public three-state contract used by the
// `baha status` path (including the 0428-078 identity_dashboard_eligible
// positive case, which is derived from the "verified" surface).
//
// It is the manifest + probe aware counterpart of the pure mapper
// capability.PersesStatusNow: it takes the resolved manifest and a context so
// it can run the probe, whereas the pure mapper takes only a selection boolean
// and therefore cannot. This entry point lives in the metrics package because
// the probe already imports both application and capability, and capability
// importing metrics would cycle.
//
// The probe is the established per-target instrument: the production path runs
// real HTTP round trips (the httpClientSeriesProbe.querySeries path, keyed on
// THIS manifest's selected Perses surface, pinned release, authorized metrics
// policy and materialized Prometheus sources) — per-target and per-provider,
// never the static reference-catalog scan that has no target/provider input.
//
// Fail-closed contract:
//   - unselected surface                 -> not_configured
//   - selected but release invalid       -> not_configured (fail-closed)
//   - selected + pinned + backend answers -> verified (probe observed a live answer)
//   - selected + pinned + silent/absent/denied/error -> configured_unverified
//
// A selected, pinned surface with a silent or not-probed target resolves to
// configured_unverified and NEVER to a static "verified"; "verified" is a
// probe result only, so pin validation alone cannot promote it. The
// verified-only predicate in perses_selection.go stays untouched.
func PersesBackendAnswersStatusNow(ctx context.Context, m application.Manifest) *capability.PersesStatus {
	// Selection and release guards FIRST: an unselected surface or an
	// invalid/unpinned release fails closed to not_configured, and this MUST
	// win over any endpoint-setup error that would otherwise resolve a
	// selected+pinned surface to configured_unverified.
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return persesStatusNotConfigured()
	}
	if capability.ValidatePersesRelease(capability.PersesDefaultRelease) != nil {
		return persesStatusNotConfigured()
	}
	// The one verifier for this surface: BuildPersesInfraDashboardWithProbe
	// answers every panel's own series query over the materialized Prometheus
	// endpoint and fails closed on any missing datasource, endpoint or
	// authorization — the same VerifyPersesInfraDashboard contract that
	// app_status_result.go names (cmd/baha/app_status_result.go:307). There is
	// no separate "real probe" definition for the three-state status: this is
	// the one.
	files, _, err := persesPrometheusDatasourceEndpoint(m)
	if err != nil {
		return persesStatusConfiguredUnverified()
	}
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return persesStatusConfiguredUnverified()
	}
	return persesBackendAnswersStatusWithProbe(ctx, m, httpClientSeriesProbe{client: client})
}

// persesBackendAnswersStatusWithProbe is the probe-injectable core of
// PersesBackendAnswersStatusNow. Production passes the real httpClientSeriesProbe
// (real HTTP round trips against the materialized per-target endpoint); tests
// pass a scripted per-target probe so an answering target and a silent target
// both exercise this exact entry point and its fail-closed mapping.
func persesBackendAnswersStatusWithProbe(ctx context.Context, m application.Manifest, probe persesInfraProbe) *capability.PersesStatus {
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return persesStatusNotConfigured()
	}
	if capability.ValidatePersesRelease(capability.PersesDefaultRelease) != nil {
		return persesStatusNotConfigured()
	}
	// The one verifier for this surface: BuildPersesInfraDashboardWithProbe
	// answers every panel's own series query over the materialized Prometheus
	// endpoint and fails closed on any missing datasource, endpoint or
	// authorization — the same VerifyPersesInfraDashboard contract named in
	// cmd/baha/app_status_result.go. No third "real probe" definition stays in
	// the codebase for this surface.
	result, err := BuildPersesInfraDashboardWithProbe(m, probe)
	if err != nil {
		return persesStatusConfiguredUnverified()
	}
	if result.MissingSignalCount() != 0 || len(result.Signals) == 0 {
		return persesStatusConfiguredUnverified()
	}
	return persesStatusVerified()
}

// persesStatusNotConfigured and persesStatusConfiguredUnverified and
// persesStatusVerified are pointer constructors for the three-state contract.
func persesStatusNotConfigured() *capability.PersesStatus {
	s := capability.PersesStatusNotConfigured
	return &s
}

func persesStatusConfiguredUnverified() *capability.PersesStatus {
	s := capability.PersesStatusConfiguredUnverified
	return &s
}

func persesStatusVerified() *capability.PersesStatus {
	s := capability.PersesStatusVerified
	return &s
}
