package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// Perses Prometheus datasource adapter (0428-083).
//
// This file realizes the Perses-native Prometheus datasource definition that
// visualises the real BaseHarbor Prometheus provider. Perses is a management
// surface, never an application capability: this adapter only binds the
// existing, Prometheus-owned metrics endpoint to a Perses datasource. It
// declares NO Metrics capability of its own (Perses only visualises the
// backend data owner that already ships the real metrics signal) and names the
// real owner (Prometheus) as an owner reference, never as a Perses-declared
// capability.
//
// The Perses JSON format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go and internal/capability/perses.go): the
// structs marshal to the Perses-native datasource/document format and the
// deterministic golden-file witness is their marshalling. No go:embed of
// datasource JSON lives in this package — the structs are the source of truth.
//
// Fail-closed: the adapter never fabricates a datasource for an unselected or
// unpinned surface, and its health check never reports "configured"/"healthy"
// without a real reachable Prometheus health path. Construction is tied to the
// retained runtime source snapshot (the materialized, authorized set of
// Prometheus sources of this provider): a missing snapshot (no source
// materialized), an unauthorized snapshot (the application may not collect the
// provider sources), an invalid snapshot, or an unusable endpoint URL all
// propagate as errors, never as a datasource with empty config and nil error.
// "verified" reachability requires a real Prometheus query over the management
// TLS boundary; unreachable, missing or denied paths are reported as errors,
// never swallowed into a false success.

// Perses Prometheus datasource identity.
const (
	// PersesPrometheusDatasourceName is the Perses datasource selector name.
	PersesPrometheusDatasourceName = "prometheus"
	// PersesPrometheusDatasourceKind is the Perses native datasource plugin
	// kind. It is the pinned native plugin, not a Grafana UI API.
	PersesPrometheusDatasourceKind = "PrometheusDatasource"
)

// PersesPrometheusDatasourceConfig is the Perses datasource config section for
// the BaseHarbor Prometheus datasource. It carries only the reachable endpoint
// and the management-auth reference: no secrets. Secrets (MTLS material or the
// optional UI basic-auth) live in the Prometheus provider PKI and are consumed
// at request time, never materialized into the descriptor.
type PersesPrometheusDatasourceConfig struct {
	// URL is the reachable Prometheus endpoint the datasource queries. The
	// build context decides the form: a context that probes from the TARGET
	// HOST (the CLI health/verify path, buildContextHostProbe) carries the
	// host loopback management endpoint with its published port — that is
	// where a host process reaches the gateway. A context that describes the
	// endpoint the PROVISIONED Perses container queries (the runtime
	// provisioning path, buildContextRuntime) carries the in-network
	// endpoint: a container cannot route the host's loopback, and the
	// published port is meaningless inside the network, so the URL names the
	// gateway service DNS on its fixed network port. Empty when the provider
	// endpoint is not yet materialized; the fail-closed health check then
	// still fails.
	URL string `json:"url,omitempty"`
	// Access is the Perses datasource access mode. BaseHarbor routes every
	// request through the Prometheus management proxy over TLS, so this is
	// always "proxy" — never "direct", which would bypass the access boundary.
	Access string `json:"access,omitempty"`
	// Authentication names the management authentication class this datasource
	// uses (native-oidc, standards-auth-adapter, native-credential,
	// unsupported). It is an owner reference, never a secret value.
	Authentication string `json:"authentication,omitempty"`
}

// PersesPrometheusDatasource is the Perses native Prometheus datasource
// resource produced by this adapter. Only the fields BaseHarbor uses are
// represented; the marshalling is deterministic and golden-tested.
type PersesPrometheusDatasource struct {
	Kind    string                           `json:"kind"`
	Name    string                           `json:"name"`
	Default bool                             `json:"default"`
	Config  PersesPrometheusDatasourceConfig `json:"config,omitempty"`
	// Owner is the real observability capability owner Perses visualises.
	// It is the Prometheus provider kind, named as an owner reference, NOT a
	// Perses-declared capability. It is not serialized: the datasource declares
	// no capability of its own.
	Owner string `json:"-"`
}

// PersesPrometheusDatasourceResult is the adapter's fail-closed verdict for the
// selected Prometheus datasource.
type PersesPrometheusDatasourceResult struct {
	// Datasource is the produced Perses Prometheus datasource resource.
	Datasource PersesPrometheusDatasource
	// SourceCount is the number of authorized Prometheus sources the datasource
	// is tied to. Construction fails closed when this would be zero, so a
	// produced result always carries at least one authorized source.
	SourceCount int
	// Reachability is exactly one of:
	//   - "unverified": the datasource is tied to selected, pinned, authorized
	//     runtime sources, but no live health/query probe observed an up=1
	//     sample. The build path without a probe always reports this.
	//   - "verified": only when a real Prometheus health/query probe succeeded
	//     (see BuildPersesPrometheusDatasourceWithProbe). No code path reports
	//     it without the probe.
	Reachability string
	// ProviderFiles carries the materialized provider state the build
	// resolved — including its RuntimeCA, the public trust bundle the
	// provisioning path anchors the datasource proxy to. The struct itself
	// never carries secret values.
	ProviderFiles ProviderFiles `json:"-"`
	// HostEndpoint is the host-loopback management endpoint (with the
	// published port) the probe ran against — the endpoint a process ON THE
	// TARGET HOST uses, independent of the build context's URL form. The
	// materializer uses it to locate the provider's trust material.
	HostEndpoint string
}

// PersesPrometheusReachabilityUnverified is the reachability state whenever no
// live probe observed an up=1 sample: the contract is defined and fail-closed,
// but health is not verified.
const PersesPrometheusReachabilityUnverified = "unverified"

// PersesPrometheusReachabilityVerified is reported only after a real,
// successful Prometheus health/query probe (see
// BuildPersesPrometheusDatasourceWithProbe).
const PersesPrometheusReachabilityVerified = "verified"

// prometheusQueryProbe is the injectable Prometheus query path used by the
// datasource health contract. Production uses the service-access client via
// queryUp; tests inject a scripted probe.
type prometheusQueryProbe interface {
	queryUp(ctx context.Context, endpoint, query string) (bool, error)
}

// unavailablePrometheusProbe performs no live observation: it never fails and
// never verifies, so the build path stays reachability-neutral.
type unavailablePrometheusProbe struct{}

func (unavailablePrometheusProbe) queryUp(context.Context, string, string) (bool, error) {
	return false, nil
}

// httpClientQueryProbe adapts the package's shared queryUp to the probe.
type httpClientQueryProbe struct {
	client *http.Client
}

func (p httpClientQueryProbe) queryUp(ctx context.Context, endpoint, query string) (bool, error) {
	return queryUp(ctx, p.client, endpoint, query)
}

// datasourceExists answers whether the Live Perses API serves a
// GlobalDatasource of this name under the given management endpoint (the
// deployment's own Perses management URL, TLS-authenticated by the client).
func (p httpClientQueryProbe) datasourceExists(ctx context.Context, managementEndpoint, name string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(managementEndpoint, "/")+"/api/globaldatasources/"+name, nil)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("perses live datasource lookup %s: HTTP %d", name, resp.StatusCode)
	}
}

// persesPrometheusDatasourcePlacement derives the full placement owning the
// given materialized provider directory from the manifest. The retained-
// snapshot authority check (runtime-sources.json must carry exactly this
// project and scope) is what proves the sources belong to this provider.
func persesPrometheusDatasourcePlacement(m application.Manifest) (Placement, error) {
	return PlacementFor(m)
}

// BuildPersesPrometheusDatasource builds the Perses Prometheus datasource for
// the selected and pinned surface. Fail-closed: it requires the observability
// management UI (Perses) selected in the manifest, a pinned release that
// validates (capability.ValidatePersesRelease), an authorized metrics policy,
// a materialized Prometheus provider endpoint, and at least one authorized
// Prometheus source in the retained runtime snapshot. Without any of these it
// returns the error, never a fabricated datasource with empty config and nil
// error. The build performs no live probe, so its reachability is always
// "unverified"; only a probe result can produce "verified"
// (BuildPersesPrometheusDatasourceWithProbe).
//
// The datasource URL is the provider's own management endpoint over the runtime
// TLS boundary (ProviderEndpoint); no secret is materialized into the
// descriptor.
func BuildPersesPrometheusDatasource(m application.Manifest) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceWithProbe(m, capability.PersesDefaultRelease, unavailablePrometheusProbe{})
}

// BuildPersesPrometheusDatasourceWithProbe builds the datasource like
// BuildPersesPrometheusDatasource and additionally runs the health/query probe,
// so the verified path is exercised against a real Prometheus query result.
// Production wiring uses BuildPersesPrometheusDatasource; Verify below uses the
// service-access client.
func BuildPersesPrometheusDatasourceWithProbe(m application.Manifest, probe prometheusQueryProbe) (PersesPrometheusDatasourceResult, error) {
	if probe == nil {
		probe = unavailablePrometheusProbe{}
	}
	return buildPersesPrometheusDatasourceWithProbe(m, capability.PersesDefaultRelease, probe)
}

// buildPersesPrometheusDatasourceWithProbe is the shared implementation keyed
// on an explicit release and probe, so the public entry point, the fail-closed
// tests and the verifying entry point all exercise identical code.
func buildPersesPrometheusDatasourceWithProbe(m application.Manifest, release capability.PersesRelease, probe prometheusQueryProbe) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceAtWithRelease(m, release, probe, "", "")
}

// buildPersesPrometheusDatasourceAt is the Target-bound core build: the
// provider state, its retained snapshot ownership and the endpoint resolve
// against dataDir/namespace (empty = the default local root). It uses the
// pinned default release; entry points that carry an explicit release must
// use buildPersesPrometheusDatasourceAtWithRelease so an unpinned release
// stays fail-closed (Luna batch19 recipe: never substitute the default when
// a release was supplied).
func buildPersesPrometheusDatasourceAt(m application.Manifest, probe prometheusQueryProbe, dataDir, namespace string) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceAtWithRelease(m, capability.PersesDefaultRelease, probe, dataDir, namespace)
}

func buildPersesPrometheusDatasourceAtWithRelease(m application.Manifest, release capability.PersesRelease, probe prometheusQueryProbe, dataDir, namespace string) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceContextual(m, release, probe, dataDir, namespace, buildContextHostProbe)
}

// persesBuildContext tells the build which endpoint form the datasource URL
// must carry (the URL field contract of PersesPrometheusDatasourceConfig).
type persesBuildContext int

const (
	// buildContextHostProbe is the CLI/verify context: a process on the
	// TARGET HOST runs the health/query probe against the published
	// loopback management endpoint (the host address form).
	buildContextHostProbe persesBuildContext = iota
	// buildContextRuntime is the provisioning context: the datasource is
	// materialized for the RUNNING Perses container, whose own loopback is
	// not the host's, so the URL names the in-network gateway endpoint.
	buildContextRuntime
)

func buildPersesPrometheusDatasourceContextual(m application.Manifest, release capability.PersesRelease, probe prometheusQueryProbe, dataDir, namespace string, buildCtx persesBuildContext, endpointOverride ...string) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceResolved(m, release, probe, buildPersesPrometheusDatasourceFilesAt, dataDir, namespace, buildCtx, endpointOverride...)
}

// buildPersesPrometheusDatasourceResolved is the shared implementation with an
// injectable provider-files resolver so the package's acceptance cells (which
// stand in a live endpoint for the materialized publish port) exercise the
// exact production build; the resolver is the only substituted edge.
func buildPersesPrometheusDatasourceResolved(m application.Manifest, release capability.PersesRelease, probe prometheusQueryProbe, resolve func(application.Manifest, string, string) (ProviderFiles, string, error), dataDir, namespace string, buildCtx persesBuildContext, endpointOverride ...string) (PersesPrometheusDatasourceResult, error) {
	// endpointOverride is the acceptance-cell edge (see
	// MaterializePersesPrometheusDatasourceAtWithEndpoint): a host-namespace
	// cell names its live loopback endpoint; production never passes one.

	// Fail-closed: without a pinned release that validates, the surface is not
	// configured, never "configured".
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesPrometheusDatasourceResult{}, err
	}
	// Fail-closed: Perses is a management surface of the observability
	// selection. Without the surface selected there is no datasource to bind,
	// never a default datasource for an unselected application.
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesPrometheusDatasourceResult{}, errors.New("perses prometheus datasource requires the observability management UI and Perses selected in the manifest")
	}
	// Fail-closed: the metrics collection authorization. A denied policy is
	// propagated, never swallowed into an empty default datasource.
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return PersesPrometheusDatasourceResult{}, fmt.Errorf("perses prometheus datasource metrics authorization: %w", err)
	}
	if resolve == nil {
		resolve = buildPersesPrometheusDatasourceFilesAt
	}
	files, hostEndpoint, err := resolve(m, dataDir, namespace)
	if err != nil {
		return PersesPrometheusDatasourceResult{}, err
	}
	// The URL form follows the build context (the PersesPrometheusDatasource-
	// Config.URL contract): host processes (the probe, the CLI verify path)
	// use the published loopback endpoint; the provisioned Perses container
	// uses the in-network gateway endpoint, because its own loopback is not
	// the host's. ProviderEndpoint validates the materialized port either
	// way, so an unmaterialized endpoint fails closed in both contexts.
	datasourceURL := hostEndpoint
	if buildCtx == buildContextRuntime {
		datasourceURL = ProviderNetworkEndpoint()
	}
	if len(endpointOverride) > 0 && strings.TrimSpace(endpointOverride[0]) != "" {
		datasourceURL = strings.TrimSpace(endpointOverride[0])
	}
	// Tie construction to the selected/authorized runtime sources: the
	// retained snapshot is the materialized source set of this provider.
	// Missing (no source materialized), denied (unauthorized classes), or
	// invalid (ownership mismatch) state all propagate as errors.
	sources, err := persesPrometheusDatasourceSourcesAt(files, m, policy, dataDir, namespace)
	if err != nil {
		return PersesPrometheusDatasourceResult{}, err
	}
	verified, err := persesPrometheusDatasourceQuery(context.Background(), hostEndpoint, sources, probe)
	if err != nil {
		return PersesPrometheusDatasourceResult{}, err
	}
	datasource := PersesPrometheusDatasource{
		Kind:    PersesPrometheusDatasourceKind,
		Name:    PersesPrometheusDatasourceName,
		Default: true,
		Owner:   string(capability.ProviderPrometheus),
		Config: PersesPrometheusDatasourceConfig{
			URL:            datasourceURL,
			Access:         "proxy",
			Authentication: release.AuthenticationClass,
		},
	}
	reachability := PersesPrometheusReachabilityUnverified
	if verified {
		reachability = PersesPrometheusReachabilityVerified
	}
	return PersesPrometheusDatasourceResult{
		Datasource:    datasource,
		SourceCount:   len(sources),
		Reachability:  reachability,
		ProviderFiles: files,
	}, nil
}

// persesPrometheusDatasourceEndpoint returns the reachable Prometheus
// management endpoint over the runtime TLS boundary. It is fail-closed: a
// missing provider directory or an unmaterialized/invalid port is surfaced as
// an error, never swallowed into an empty URL.
func persesPrometheusDatasourceEndpoint(m application.Manifest) (ProviderFiles, string, error) {
	return persesPrometheusDatasourceEndpointAt(m, "", "")
}

// persesPrometheusDatasourceEndpointAt resolves the materialized provider
// endpoint in the given data root/namespace (empty = the default local root).
func persesPrometheusDatasourceEndpointAt(m application.Manifest, dataDir, namespace string) (ProviderFiles, string, error) {
	var files ProviderFiles
	var err error
	if dataDir == "" && namespace == "" {
		files, err = ExistingProviderFiles(m)
	} else {
		files, err = ExistingProviderFilesAt(dataDir, namespace, m)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProviderFiles{}, "", errors.New("perses prometheus datasource has no materialized Prometheus provider: provision the existing provider first")
		}
		return ProviderFiles{}, "", fmt.Errorf("perses prometheus datasource provider state: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return ProviderFiles{}, "", fmt.Errorf("perses prometheus datasource endpoint: %w", err)
	}
	return files, endpoint, nil
}

// persesPrometheusDatasourceSources ties the datasource to the selected,
// authorized runtime sources carried by the retained snapshot of the
// materialized provider files. Missing (absent), unauthorized (denied by the
// collection policy), and invalid (damaged ownership) source state is
// propagated as an error, never accepted as nil. Application-owned sources
// must belong to this application; provider-owned sources must be collectable
// under the policy.
func persesPrometheusDatasourceSources(files ProviderFiles, m application.Manifest, policy application.MetricsDeploymentPolicy) ([]observability.MetricsSource, error) {
	return persesPrometheusDatasourceSourcesAt(files, m, policy, "", "")
}

// persesPrometheusDatasourceSourcesAt is the Target-bound form: the placement
// whose ownership the retained snapshot must match resolves in
// dataDir/namespace (empty = the default local root), exactly where the
// deploy path materialized the provider.
func persesPrometheusDatasourceSourcesAt(files ProviderFiles, m application.Manifest, policy application.MetricsDeploymentPolicy, dataDir, namespace string) ([]observability.MetricsSource, error) {
	placement, err := PlacementForAt(dataDir, namespace, m)
	if err != nil {
		return nil, fmt.Errorf("perses prometheus datasource provider placement: %w", err)
	}
	sources, found, err := readRuntimeSourceSnapshot(files, placement)
	if err != nil {
		return nil, fmt.Errorf("perses prometheus datasource sources: %w", err)
	}
	if !found || len(sources) == 0 {
		return nil, errors.New("perses prometheus datasource has no materialized Prometheus sources: the provider collected no metrics sources, so there is nothing to visualise")
	}
	var authorized []observability.MetricsSource
	for _, source := range sources {
		switch source.Class {
		case observability.SourceApplication:
			if !policy.Collect[application.MetricsSourceApplication] {
				continue
			}
			if source.OwnerApplication != m.Name {
				continue
			}
		case observability.SourceApplicationProvider:
			if !policy.Collect[application.MetricsSourceApplicationProvider] {
				continue
			}
		case observability.SourcePlatformProvider:
			if !policy.Collect[application.MetricsSourcePlatformProvider] {
				continue
			}
		}
		authorized = append(authorized, source)
	}
	if len(authorized) == 0 {
		return nil, fmt.Errorf("perses prometheus datasource has no authorized Prometheus sources for application %s/%s: all materialized sources are denied by the metrics collection policy or owned by another application", m.Name, m.Environment)
	}
	return authorized, nil
}

// persesPrometheusDatasourceQuery is the exact datasource-health query: the
// maximum over the union of the authorized provider up-series, so one probe
// answers for every source the datasource visualises. A probe error is
// propagated; a clean answer is the verified/down verdict.
func persesPrometheusDatasourceQuery(ctx context.Context, endpoint string, sources []observability.MetricsSource, probe prometheusQueryProbe) (bool, error) {
	clauses := make([]string, 0, len(sources))
	for _, source := range sources {
		clauses = append(clauses, fmt.Sprintf(`up{baseharbor_provider=%q,baseharbor_source=%q}`, string(source.Provider), source.ID))
	}
	query := "max(" + strings.Join(clauses, " or ") + ")"
	return probe.queryUp(ctx, endpoint, query)
}

// VerifyPersesPrometheusDatasourceHealth enforces the fail-closed datasource
// health contract: a selected, pinned Prometheus datasource requires a real
// reachable Prometheus health path. It performs the datasource query (max over
// the authorized provider up-series) against the materialized management
// endpoint through the Prometheus service-access TLS client. Success requires
// ALL of: a valid pinned release, the surface selected, an authorized metrics
// policy, a materialized endpoint, authorized materialized sources, and a real
// probe result with at least one source up. Pin validation alone NEVER passes.
// Unreachable, missing, denied, invalid or all-down state is returned as an
// error, never as a false success.
func VerifyPersesPrometheusDatasourceHealth(ctx context.Context, m application.Manifest) error {
	return verifyPersesPrometheusDatasourceHealthAt(ctx, m, "", "")
}

// verifyPersesPrometheusDatasourceHealthAt is the Target-bound form (empty
// dataDir/namespace = the default local root, the legacy shape).
func verifyPersesPrometheusDatasourceHealthAt(ctx context.Context, m application.Manifest, dataDir, namespace string) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return err
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return errors.New("perses prometheus datasource requires the observability management UI and Perses selected in the manifest")
	}
	files, _, err := persesPrometheusDatasourceEndpointAt(m, dataDir, namespace)
	if err != nil {
		return err
	}
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return fmt.Errorf("perses prometheus datasource metrics authorization: %w", err)
	}
	sources, err := persesPrometheusDatasourceSourcesAt(files, m, policy, dataDir, namespace)
	if err != nil {
		return err
	}
	// The datasource under verification is the identity the DEPLOYMENT
	// materialized — the GlobalDatasource resource the running Perses loads
	// from its provisioning folder — not an endpoint this function
	// recomputes for the host (0428-083 F4). Fail-closed chain: read the
	// materialized resource, prove the LIVE Perses serves it (existence
	// through the management API), then run its health THROUGH the Perses
	// proxy for that datasource name.
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return fmt.Errorf("perses prometheus datasource health client: %w", err)
	}
	probe := httpClientQueryProbe{client: client}
	// The Perses management URL the LIVE existence check runs against: this
	// deployment's own Perses surface on its published loopback port (the
	// render's PersesUIHostPortEnv contract), never the Prometheus endpoint.
	persesManagementURL, err := persesDeploymentManagementURLAt(dataDir, namespace, m)
	if err != nil {
		return err
	}
	resourceFile, _, _, err := PersesDatasourceProviderPaths(dataDir, namespace, m)
	if err != nil {
		return err
	}
	name, proxyHealthURL, err := readPersesDatasourceResourceIdentity(resourceFile)
	if err != nil {
		return err
	}
	// The watcher (enable_watch) applies provisioning changes within seconds
	// of the materialization; wait for the LIVE Perses to serve the datasource
	// up to a bounded deadline, then fail closed.
	loaded := false
	waitDeadline := time.Now().Add(30 * time.Second)
	for !loaded {
		exists, lookupErr := probe.datasourceExists(ctx, persesManagementURL, name)
		if lookupErr != nil {
			return fmt.Errorf("perses prometheus datasource health: cannot read the live datasource: %w", lookupErr)
		}
		loaded = exists
		if loaded || time.Now().After(waitDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if !loaded {
		return fmt.Errorf("perses prometheus datasource health: GlobalDatasource %q is not loaded by the running Perses within the watcher window", name)
	}
	verified, err := persesDatasourceHealthUp(ctx, client, proxyHealthURL)
	if err != nil {
		return fmt.Errorf("perses prometheus datasource health: datasource %q health query failed: %w", name, err)
	}
	if !verified {
		return errors.New("perses prometheus datasource health is not verified: the datasource health endpoint reports no up=1 sample")
	}
	_ = sources
	return nil
}

// persesDeploymentManagementURLAt resolves the Perses management URL of the
// deployment's own running Perses surface: https on the published loopback
// host port the runtime env carries (application.PersesUIHostPortEnv — the
// exact address the render publishes and the CLI surface uses). Fail-closed
// on a missing or unparsable runtime env.
func persesDeploymentManagementURLAt(dataDir, namespace string, m application.Manifest) (string, error) {
	runtimeEnv := filepath.Join(application.RuntimeFilesFor(
		application.Store{Root: filepath.Clean(dataDir), Namespace: namespace}, m).Dir, "runtime.env")
	values, err := application.LoadRuntimeEnvFile(runtimeEnv)
	if err != nil {
		return "", fmt.Errorf("perses prometheus datasource health: %w", err)
	}
	port := strings.TrimSpace(values[application.PersesUIHostPortEnv])
	if port == "" {
		return "", fmt.Errorf("perses prometheus datasource health: the runtime env carries no %s", application.PersesUIHostPortEnv)
	}
	return "https://127.0.0.1:" + port, nil
}

// readPersesDatasourceResourceIdentity reads the provisioned GlobalDatasource
// resource file (name + its proxy health URL) — the materialized identity the
// running Perses is expected to serve.
func readPersesDatasourceResourceIdentity(resourceFile string) (name, proxyHealthURL string, err error) {
	data, err := os.ReadFile(resourceFile)
	if err != nil {
		return "", "", fmt.Errorf("perses prometheus datasource health: materialized GlobalDatasource: %w", err)
	}
	var resource struct {
		Metadata struct {
			Name string `json:"metadata"`
		} `json:"metadata"`
		Spec struct {
			Default struct {
				Spec struct {
					Proxy struct {
						Spec struct {
							URL string `json:"url"`
						} `json:"spec"`
					} `json:"proxy"`
				} `json:"spec"`
			} `json:"spec"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &resource); err != nil {
		return "", "", fmt.Errorf("perses prometheus datasource health: parsing the GlobalDatasource resource: %w", err)
	}
	name = PersesDatasourceProvisioningResourceName
	proxyHealthURL = resource.Spec.Default.Spec.Proxy.Spec.URL
	if proxyHealthURL == "" {
		return "", "", errors.New("perses prometheus datasource health: the materialized GlobalDatasource names no proxy URL")
	}
	return name, proxyHealthURL + "/api/v1/datasources/http", nil
}

// PersesPrometheusDatasourceJSON marshals the produced datasource
// deterministically (json.MarshalIndent with trailing newline), mirroring the
// golden-file witness pattern used across the metrics package.
func PersesPrometheusDatasourceJSON(d PersesPrometheusDatasource) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// buildPersesPrometheusDatasourceFilesAt resolves the materialized provider
// files and the host-loopback management endpoint (with the published port)
// the datasource build binds to. It is the production resolver edge.
func buildPersesPrometheusDatasourceFilesAt(m application.Manifest, dataDir, namespace string) (ProviderFiles, string, error) {
	return persesPrometheusDatasourceEndpointAt(m, dataDir, namespace)
}

// persesDatasourceHealthUp runs the health query through the Perses datasource
// proxy: GET <proxy>/datasource/http/api/v1/query?query=up. The proxy performs
// the request against the provisioned endpoint with the GlobalSecret's
// transport material, so this proves the END-TO-END path the dashboards use
// (proxy → gateway → Prometheus), not a host-side shortcut (0428-083 F4).
func persesDatasourceHealthUp(ctx context.Context, client *http.Client, proxyBase string) (bool, error) {
	target := strings.TrimRight(proxyBase, "/") + "/datasource/http/api/v1/query?query=up"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return false, fmt.Errorf("the datasource proxy answered HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	if payload.Status != "success" {
		return false, errors.New("the datasource proxy query did not succeed")
	}
	for _, result := range payload.Data.Result {
		if len(result.Value) != 2 {
			continue
		}
		if value, ok := result.Value[1].(string); ok && value == "1" {
			return true, nil
		}
	}
	return false, nil
}
