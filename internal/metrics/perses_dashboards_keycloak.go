package metrics

// PersesDatasourceKeycloak is the Perses datasource plugin kind for the
// Keycloak identity dashboard. Keycloak exposes its native openmetrics
// endpoint, so it is modelled as a PrometheusDatasource (keyed to the
// registered keycloak target) rather than a dedicated plugin kind.
const PersesDatasourceKeycloak = "PrometheusDatasource"

// persesPanelKeycloak is the BaseHarbor Keycloak identity dashboard panel set.
// Keycloak is the identity provider, not a metrics/logs/traces backend, so this
// is the ONLY dashboard whose backend is DashboardBackendIdentity. It scrapes
// Keycloak's documented, native openmetrics endpoint (reference.go:
// keycloak-metrics, Mode native, Protocol openmetrics, Verification
// ObservabilityVerifyBackend) with a PrometheusDatasource keyed to the
// registered keycloak target.
//
// Keycloak is a PROVIDER, so its production target is a provider-class source
// (internal/metrics/prometheus_files.go targetGroupPayload: job="baseharbor-
// providers", baseharbor_provider="keycloak", baseharbor_source,
// baseharbor_source_class, baseharbor_metrics_path) with NO baseharbor_
// application/environment label — those belong to application-class sources
// (job="baseharbor-applications"). Every selector therefore keys on the real
// provider-class contract only; a keycloak_application/baseharbor_environment
// selector could never match a real Keycloak series. The queries reference only
// documented, concretely-evidenced HTTP-server metrics
// (http_server_requests_seconds_count, http_server_requests_seconds_sum,
// http_server_active_requests); no fabricated or undocumented metric ships.
func persesPanelKeycloak() PersesPanel {
	return PersesPanel{
		Kind: PersesPanelKind,
		Spec: PersesPanelSpec{
			Display: PersesDisplay{Name: "BaseHarbor Keycloak identity"},
			Plugin:  PersesPanelPlugin{Kind: PersesPanelPluginTimeSeriesChart, Spec: mustJSON(map[string]any{})},
			Queries: []PersesQueryWrapper{{
				Kind: PersesQueryWrapperTimeSeries,
				Spec: PersesQuerySpec{
					Plugin: PersesQueryPlugin{
						Kind: PersesQueryPrometheus,
						Spec: mustJSON(map[string]any{
							"query": `http_server_requests_seconds_count{job="baseharbor-providers",baseharbor_provider="keycloak"}`,
						}),
					},
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourceKeycloak, Name: "keycloak"},
				},
			}, {
				Kind: PersesQueryWrapperTimeSeries,
				Spec: PersesQuerySpec{
					Plugin: PersesQueryPlugin{
						Kind: PersesQueryPrometheus,
						Spec: mustJSON(map[string]any{
							"query": `http_server_requests_seconds_sum{job="baseharbor-providers",baseharbor_provider="keycloak"}`,
						}),
					},
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourceKeycloak, Name: "keycloak"},
				},
			}, {
				Kind: PersesQueryWrapperTimeSeries,
				Spec: PersesQuerySpec{
					Plugin: PersesQueryPlugin{
						Kind: PersesQueryPrometheus,
						Spec: mustJSON(map[string]any{
							"query": `http_server_active_requests{job="baseharbor-providers",baseharbor_provider="keycloak"}`,
						}),
					},
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourceKeycloak, Name: "keycloak"},
				},
			}},
		},
	}
}
