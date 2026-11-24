package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// errorsIsNotExist is a local alias so the file reads os.IsNotExist-agnostic
// (errors.Is(err, os.ErrNotExist)); it is the only call site needing it and it
// keeps the fail-closed "absent is correct" semantics explicit and single-line.
func errorsIsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// perses_dashboard_provision.go realises the dashboard CONSUMER of card 0428-181
// (P4 of review 1650): the Perses dashboard catalog is built (perses_dashboards.go)
// and its selection catalog + identity-selection witness are defined and unit
// tested (perses_selection.go), but nothing wrote a single dashboard through the
// apply/runtime path — the catalog had ZERO production consumers. This file wires
// the catalog into the real export path so a selected, verified identity surface
// actually PROVISIONS its dashboard to Perses instead of only reporting an
// availability verdict.
//
// PROVISION TARGET (Owner design, 0428-181). The Perses loader walks the mounted
// provisioning folder RECURSIVELY and accepts resources by their "kind", so a
// dashboard JSON under provisioning/dashboards/ loads through the EXISTING
// /etc/perses/provisioning mount with NO config.yaml change. The config builder
// (NewPersesDatasourceConfig, internal/metrics/perses_prometheus_datasource_runtime.go)
// is owned by review 1650 and stays untouched; the dashboards folder needs no
// second provisioning-folders entry because the loader already recurses it.
//
// GATE (Owner design, 0428-181). Only a SELECTED surface that the REAL
// per-target probe PersesBackendAnswersStatusNow reports VERIFIED exports any
// dashboard: a deselected or silent/not-probed/unverified surface writes nothing
// and cleans any stale dashboards (a dropped-but-unverified dashboard must never
// reach Perses). Eligible dashboard keys are chosen through the public, test
// pinned capability.FilterEligibleDashboards gate, which internally loops
// EligibleFor; the identity witness is passed so keycloak-identity fails closed
// when the four evidence flags are not all attested.
//
// WRITE-SET (0428-181). This is a NEW disjoint metrics file (AGENTS 4.3), so it
// does not touch perses_dashboards*.go, the config builder, or
// cmd/baha/app_status_result.go (all owned by review 1650): it only references
// their exported/same-package symbols.
//
// STRUCTURE (mirrors the 0428-083 datasource seam). internal/application cannot
// import internal/metrics (metrics imports application — a cycle), so the real
// probe+provision logic lives here, and internal/application/perses_runtime.go
// exposes the registration seam that INVOKES this provision step on the apply
// path plus the no-op guard when the surface is unselected.

// persesDashboardProvisioningFolder is the in-container-relative name of the
// dashboard provisioning subfolder the loader already recurses under the mounted
// /etc/perses/provisioning folder (no config.yaml entry needed).
const persesDashboardProvisioningFolder = "dashboards"

// persesDashboardProvisionForLifecycle is the apply-boundary entry the lifecycle
// seam (application.RegisterPersesDashboardProvisioner) calls: (manifest,
// application runtime dir, namespace). The runtime dir is the RuntimeFiles.Dir
// the lifecycle computed through application.RuntimeFilesFor — the one layout
// formula of this Core — so the dashboard projection root is its
// providers/management-ui/perses sibling, the SAME root the datasource
// materializer writes beside (F1): the compose render (whose context dir is that
// same runtime dir) mounts the whole provisioning tree, and the loader loads the
// dashboards/ subfolder recursively.

// persesDashboardProvisionForLifecycle is the apply-boundary entry the lifecycle
// seam calls: (manifest, application runtime dir, namespace). The runtime dir is
// the RuntimeFiles.Dir the lifecycle computed through application.RuntimeFilesFor
// — the one layout formula of this Core — so the dashboard projection root is its
// providers/management-ui/perses sibling, the SAME root the datasource
// materializer writes beside (F1): the compose render (whose context dir is that
// same runtime dir) mounts the whole provisioning tree, and the loader loads the
// dashboards/ subfolder recursively.
func persesDashboardProvisionForLifecycle(m application.Manifest, runtimeDir, namespace string) error {
	root := datasourceProjectionRoot(runtimeDir)
	// Fail-closed on the selection axis FIRST: a deselected surface must never
	// leave a stale dashboard behind a re-selected Perses would load, so the
	// whole dashboards subfolder is torn down.
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return removePersesDashboardProjection(root)
	}
	// The REAL per-target probe. A selected, pinned surface that is silent,
	// not-probed, denied or erroring resolves to configured_unverified and MUST
	// not export a dashboard: "verified" is a probe result only, so pin
	// validation alone cannot promote it (the verified-only predicate in
	// perses_selection.go stays untouched). The dashboards subfolder is removed
	// so a previously-verified surface that now stands silent exports nothing.
	status := persesBackendAnswersStatusWithProbe(context.Background(), m, persesBackendProbe())
	if status == nil || *status != capability.PersesStatusVerified {
		return removePersesDashboardProjection(root)
	}
	// The surface is verified: the prometheus backend the probe answers is a
	// real, live signal, so prometheus-targets is eligible on its own backend
	// signal. The loki/tempo/identity backends are not probed by the
	// prometheus-backed infrastructure probe, so they carry no backend signal
	// here and stay ineligible on the per-backend gate — only keycloak-identity
	// can still export, and only when the identity witness is non-forgeably
	// available.
	backendSignals := map[capability.DashboardBackend]bool{
		capability.DashboardBackendPrometheus: true,
	}
	witness := identitySelectionWitness(m)
	eligible, err := capability.FilterEligibleDashboards(capability.DefaultDashboardCatalog(), witness, backendSignals)
	if err != nil {
		// A gate error means the selection catalog itself is inconsistent;
		// fail closed rather than export an unvalidated dashboard.
		return err
	}
	return writePersesDashboardProjection(m, eligible, root)
}

// identitySelectionWitness builds the authoritative identity-selection witness
// for the resolved manifest. Availability is DERIVED from real material only,
// never a caller boolean: the identity provider is confirmed selected when the
// manifest declares it, the Keycloak native metrics signal is present and pinned
// to the current release when it is registered in the observability signal
// registry, and the Target is authorized when a real Keycloak provider placement
// exists for the deployment. Every missing flag fails closed to unavailable, so
// keycloak-identity can never export without real evidence.
// identitySelectionWitness is the lifecycle-visible seam to the identity
// witness resolver of the metrics package. It defaults to the real resolver
// (derived from real manifest + observability registry + provider placement);
// tests override it with a stub so the export witness is deterministic.
var identitySelectionWitness = func(m application.Manifest) capability.IdentitySelectionWitness {
	selectionConfirmed := m.Services.Identity || m.Services.IdentityManagementUI
	metricPresent, metricCurrent, targetAuthorized := false, false, false
	if selectionConfirmed {
		metricPresent, metricCurrent = keycloakMetricsSignalPresent(m)
		targetAuthorized = keycloakTargetAuthorized(m)
	}
	return capability.NewIdentitySelectionWitness(selectionConfirmed, metricPresent, metricCurrent, targetAuthorized)
}

// keycloakMetricsSignalPresent reports whether a real Keycloak native metrics
// signal is registered for this deployment (metric present) and pinned to the
// current accepted Perses release (metric current). An absent registration is
// the fail-closed "no real signal" verdict, so the identity dashboard stays
// ineligible.
//
// The Keycloak source of record is registered under the canonical
// application-metrics ID grammar (application:metrics:<app>:<env>:<source>),
// the SAME id the Keycloak realization books onto (0428-168): the deployment
// application, this manifest's environment and the Keycloak metrics source
// name. The witness never invents a registration — an unregistered source
// fails closed.
func keycloakMetricsSignalPresent(m application.Manifest) (present, current bool) {
	if strings.TrimSpace(m.Name) == "" {
		return false, false
	}
	// The Keycloak source of record is registered under the canonical
	// application-metrics ID grammar (application:metrics:<app>:<env>:<source>),
	// the SAME id the Keycloak realization books onto (0428-168): the
	// deployment application, this manifest's environment and the Keycloak
	// metrics source name. The witness never invents a registration — an
	// unregistered source fails closed.
	id := observability.ApplicationMetricsSourceID(m.Name, m.Environment, "keycloak-metrics")
	has, err := observability.RegistryHasSignal(id, capability.ProviderKeycloak, observability.SignalMetrics)
	if err != nil {
		return false, false
	}
	if !has {
		return false, false
	}
	// metric current: the pinned release is valid; an unpinned Perses release
	// carries no real backend signal, so the identity dashboard stays
	// ineligible (the verified-only predicate in perses_selection.go stays
	// untouched). The pinned release is the manifest's single release, so
	// manifest truth == pinned-release validity here.
	return true, capability.ValidatePersesRelease(capability.PersesDefaultRelease) == nil
}

// keycloakTargetAuthorized reports whether a real Keycloak provider placement
// exists for the deployment (the Target-authorization evidence of the witness).
// A deployment with no resolved Keycloak placement carries no authorization, so
// the witness fails closed. The resolution is the canonical single-call
// application-side lookup, the SAME one the Keycloak realization uses.
func keycloakTargetAuthorized(m application.Manifest) bool {
	_, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	return err == nil
}

// removePersesDashboardProjection tears down the dashboards subfolder of a
// deselected or unverified surface (never a stale dashboard for the next start).
// Fail-closed only on write errors: an absent folder is the correct no-op.
func removePersesDashboardProjection(root string) error {
	dir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	if err := os.RemoveAll(dir); err != nil && !errorsIsNotExist(err) {
		return err
	}
	return nil
}

// writePersesDashboardProjection writes each eligible dashboard as its native
// Perses dashboard resource under provisioning/dashboards/, keyed by catalog
// key so the loader resolves it by kind + name. It clears stale regular files of
// the previous export first (the loader loads EVERY file, so two generations
// must never coexist) and fails closed when the folder cannot be written.
func writePersesDashboardProjection(m application.Manifest, eligible []string, root string) error {
	dir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create perses dashboard provisioning folder: %w", err)
	}
	// The provisioning loader loads every regular file in the folder, so a
	// stale dashboard of a previous export (or a previously eligible key that is
	// no longer eligible) must never coexist with the current one.
	if err := clearPersesDashboardProvisioningFolder(dir); err != nil {
		return err
	}
	for _, key := range eligible {
		// The data/DB dashboards live in perses_dashboards.go (metrics
		// same-package accessor). The Keycloak identity dashboard is built in
		// THIS file (0428-181): its JSON content is identity-excluded from
		// perses_dashboards.go by design, so a catalog lookup would miss it.
		entry := persesDashboardCatalog()[key]
		if key == "keycloak-identity" {
			entry.Dashboard = persesKeycloakIdentityDashboard(m)
		}
		raw, err := persesPanelJSON(entry)
		if err != nil {
			return fmt.Errorf("marshal perses dashboard %q: %w", key, err)
		}
		path := filepath.Join(dir, key+".json")
		if err := writePersesDashboardFile(path, raw); err != nil {
			return fmt.Errorf("write perses dashboard %q: %w", key, err)
		}
	}
	return nil
}

// clearPersesDashboardProvisioningFolder removes the regular files of the
// dashboard provisioning folder (never the folder itself), so the export is the
// only content the loader sees.
func clearPersesDashboardProvisioningFolder(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errorsIsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read perses dashboard provisioning folder: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errorsIsNotExist(err) {
			return fmt.Errorf("remove stale perses dashboard %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// writePersesDashboardFile writes the dashboard with 0600 then relaxes it to the
// readable projection mode the managed-UI pattern uses (the distroless Perses
// process cannot read owner-only bind mounts): owner write first, then chmod
// 0644, like the datasource file writer beside it.
func writePersesDashboardFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

// persesDashboardProvisioner is the lifecycle seam to the metrics-side dashboard
// provisioner. It has the same signature as the datasource materializer: the
// lifecycle holds the real runtime dir, the metrics provisioner derives the
// sibling-of-runtime projection root through the ONE application-side formula.
var persesDashboardProvisioner atomic.Pointer[func(m application.Manifest, runtimeDir, namespace string) error]

// persesBackendProbe is the lifecycle-visible seam to the per-target Perses
// backend probe of the metrics package. It defaults to the real HTTP series
// probe; tests override it with a stub that answers "verified" (or not) without
// a live backend, so the export gate is deterministic and probe-observable.
var persesBackendProbe func() persesInfraProbe = realPersesBackendProbe

// RegisterPersesDashboardProvisioner registers the metrics-side dashboard
// provisioner with the application lifecycle seam. It is called once from the
// metrics package init; the seam exists because internal/metrics imports
// internal/application, so the lifecycle cannot carry a static edge in the other
// direction without an import cycle.
func RegisterPersesDashboardProvisioner(fn func(m application.Manifest, runtimeDir, namespace string) error) {
	prov := fn
	persesDashboardProvisioner.Store(&prov)
}

func init() {
	RegisterPersesDashboardProvisioner(persesDashboardProvisionForLifecycle)
}

// datasourceProjectionRoot resolves the providers/management-ui/perses sibling
// of an application runtime dir — the SAME root the datasource materializer
// writes beside (0428-083 F1), so the dashboards land beside the datasource the
// compose render mounts from. It is the one layout formula of this route and
// is derived only here, not duplicated.
func datasourceProjectionRoot(runtimeDir string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(runtimeDir)), filepath.FromSlash(application.PersesDatasourceRelativeRoot))
}

// realPersesBackendProbe is the production per-target Perses backend probe: the
// HTTP series probe over the package's management TLS client. It is the only
// real probe this package runs, mirroring the datasource verify lane.
func realPersesBackendProbe() persesInfraProbe {
	return httpClientSeriesProbe{}
}

// persesKeycloakIdentityDashboard is the Perses-native dashboard that realises
// the Keycloak identity provider for the resolved deployment. It is built in
// THIS write-set (card 0428-181), not in perses_dashboards.go (#1650/#1818):
// the observability catalog is identity-excluded BY DESIGN and its
// TestPersesDashboardKeysMatchSelectionCatalogKeys skips DashboardBackendIdentity
// on purpose. It is keyed off DefaultDashboardCatalog's keycloak-identity
// template (Backend=DashboardBackendIdentity, Datasource="keycloak") so the
// provisioned resource matches the selection-catalog key exactly, never a
// fabricated key.
//
// The dashboard binds the Keycloak identity provider with one stat panel over
// the identity signal, the same $application/$env dashboard-level variables the
// data dashboards carry, and a Prometheus datasource as the shared observability
// backend (the identity surface is only observable through the pinned Perses
// + Prometheus backend). It writes no secret, credential or token.
func persesKeycloakIdentityDashboard(m application.Manifest) PersesDashboard {
	return PersesDashboard{
		Kind:     PersesDashboardKind,
		Metadata: PersesDashboardMetadata{Name: "keycloak-identity", Version: 0},
		Spec: PersesDashboardSpec{
			Display: PersesDisplay{
				Name:        "BaseHarbor Keycloak identity",
				Description: "Identity provider selection and signal for the resolved BaseHarbor deployment.",
			},
			Datasources: map[string]PersesDatasource{
				// The observability backend the identity surface is observable
				// through (Prometheus, the pinned Perses + Prometheus backend).
				"prometheus": {Default: true, Plugin: PersesDatasourcePlugin{Kind: PersesDatasourcePrometheus, Spec: mustJSON(map[string]any{})}},
			},
			Panels: map[string]PersesPanel{
				"identity-signal": {
					Kind: PersesPanelKind,
					Spec: PersesPanelSpec{
						Display: PersesDisplay{Name: "Keycloak identity signal"},
						Plugin:  PersesPanelPlugin{Kind: PersesPanelPluginStatChart, Spec: mustJSON(map[string]any{})},
						Queries: []PersesQueryWrapper{{
							Kind: PersesQueryWrapperTimeSeries,
							Spec: PersesQuerySpec{
								Plugin:     PersesQueryPlugin{Kind: PersesQueryPrometheus, Spec: mustJSON(map[string]any{"query": `identity_selection_confirmed`})},
								Datasource: &PersesDatasourceRef{Kind: PersesDatasourcePrometheus, Name: "prometheus"},
							},
						}},
					},
				},
			},
			Layouts: []PersesLayout{
				{
					Kind: PersesGridLayoutKind,
					Spec: PersesLayoutSpec{
						Items: []PersesGridItem{{
							X: 0, Y: 0, Width: 12, Height: 6,
							Content: PersesGridContent{Ref: "#/spec/panels/identity-signal"},
						}},
					},
				},
			},
			Variables: persesDashboardVariables(m),
			Duration:  "5m",
		},
	}
}
