package metrics

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// Perses Tempo datasource adapter (0428-091).
//
// This file realizes the pinned, Perses-native Tempo datasource resource that
// visualises the real BaseHarbor Tempo provider — a management surface, never
// an application capability. See perses_tempo_contract.go for the upstream
// witness and its shape rules: REST Datasource {kind, metadata, spec} with no
// apiVersion, plugin kind TempoDatasource, panel query TraceQuery wrapping the
// TempoTraceQuery plugin. The build path is fail-closed on selection,
// ownership and authorization, and reachability is decided only by a real
// TraceQL answer over the authorized service-access client.

// Perses Tempo datasource identity: the pinned Perses REST resource envelope
// plus the pinned native Tempo plugin kinds. The REST Datasource model carries
// no apiVersion (see the header note), so this block names none.
const (
	// PersesTempoDatasourceName is the Perses datasource metadata name.
	PersesTempoDatasourceName = "baseharbor-tempo"
	// PersesTempoResourceKind is the pinned Perses REST resource kind.
	PersesTempoResourceKind = "Datasource"
	// PersesTempoProject is the Perses project the datasource resource lives in:
	// the shared BaseHarbor observability project.
	PersesTempoProject = "baseharbor"
	// PersesTempoDatasourceKind is the pinned native Perses Tempo datasource
	// plugin kind. It is the plugin the pinned release ships, never a
	// self-invented "TraceDatasource" kind and never a Grafana UI API plugin.
	PersesTempoDatasourceKind = "TempoDatasource"
	// PersesTempoTraceQueryWrapperKind is the pinned query-wrapper kind of a
	// panel trace query (spec/go/plugin/plugin.go KindTraceQuery, the module
	// perses/perses v0.55.1 requires: github.com/perses/spec v0.3.0).
	PersesTempoTraceQueryWrapperKind = "TraceQuery"
	// PersesTempoTraceQueryKind is the pinned native Perses Tempo trace-query
	// plugin kind carried inside the wrapper's spec.plugin.
	PersesTempoTraceQueryKind = "TempoTraceQuery"
	// PersesTempoHTTPProxyKind is the pinned HTTP proxy kind the datasource
	// proxy envelope declares (the proxy.HTTPProxy kind the upstream schema
	// module and dev/data witness documents use).
	PersesTempoHTTPProxyKind = "HTTPProxy"
	// PersesTempoTraceQueryPath is the Tempo TraceQL search path the datasource
	// proxy forwards to: the search endpoint of the Tempo query frontend.
	PersesTempoTraceQueryPath = "/api/search"
)

// tempoTraceQueryTimeout bounds one real trace-query probe. It is a compile-time
// alias of the shared application StatusTimeoutMatrix FastServiceTimeout group
// (internal/application/timeouts.go): the Tempo trace-query probe is the same
// 10s probe budget the CLI fast-service probes use, so every consumer reads one
// documented value from one source of truth (H-V42, 0428-315) instead of a
// duplicated private literal. Declared as a const, not a var, so no package
// code can mutate it at runtime and the matrix aliasing is a compile-time
// invariant (Codex re-review correction). The value is unchanged (10s), so
// behaviour is preserved exactly — this is a consolidation, not a tuning change.
// The matrix group is a typed time.Duration constant, so this alias keeps that
// exact type.
const tempoTraceQueryTimeout = application.FastServiceTimeout

// tempoTraceQueryTimeoutMatrixAlias is the static drift guard for the matrix
// alias: its length is the CONSTANT difference alias − matrix. A mutable var
// alias makes the length non-constant and a downward drift makes it negative;
// either one fails the build.
type tempoTraceQueryTimeoutMatrixAlias [tempoTraceQueryTimeout - application.FastServiceTimeout]struct{}

// tempoTraceQueryTimeoutMatrixAliasExact is the upward half of the guard: the
// same difference taken the other way round (matrix − alias), so an upward
// drift gives a negative length and a mutation a non-constant length; both
// fail the build.
type tempoTraceQueryTimeoutMatrixAliasExact [application.FastServiceTimeout - tempoTraceQueryTimeout]struct{}

// assertTempoAliasMatrixEquality is the referenced assertion: taking its type
// (TestTempoTraceQueryTimeoutIsCompileTimeConstant) forces the compiler to
// evaluate both guards; each has size 0 exactly while the alias is a
// compile-time constant equal to the matrix group value. Mutation (array
// length not constant) and drift in either direction (array length negative)
// therefore break compilation, not just a point-in-time test. The package
// value tests keep the runtime equality assertion as well (Codex re-review
// fix).
func assertTempoAliasMatrixEquality() (tempoTraceQueryTimeoutMatrixAlias, tempoTraceQueryTimeoutMatrixAliasExact) {
	return tempoTraceQueryTimeoutMatrixAlias{}, tempoTraceQueryTimeoutMatrixAliasExact{}
}

// PersesTempoResource is the native Perses Datasource resource BaseHarbor
// registers for the Tempo backend. kind/metadata/spec is the exact Perses
// v0.55.1 REST model (pkg/model/api/v1/datasource.go declares no apiVersion
// field); spec.plugin carries the native TempoDatasource plugin.
type PersesTempoResource struct {
	Kind     string                  `json:"kind"`
	Metadata PersesTempoMetadata     `json:"metadata"`
	Spec     PersesTempoResourceSpec `json:"spec"`
}

// PersesTempoMetadata is the Perses resource metadata of a project-scoped
// datasource.
type PersesTempoMetadata struct {
	Name    string `json:"name"`
	Project string `json:"project"`
}

// PersesTempoResourceSpec is the spec section of the datasource resource.
type PersesTempoResourceSpec struct {
	Default bool                 `json:"default"`
	Plugin  PersesTempoPluginRef `json:"plugin"`
}

// PersesTempoPluginRef is the plugin reference inside a datasource spec.
type PersesTempoPluginRef struct {
	Kind string                `json:"kind"`
	Spec PersesTempoPluginSpec `json:"spec"`
}

// PersesTempoPluginSpec is the TempoDatasource plugin spec. DirectURL is
// deliberately omitted: BaseHarbor never lets the browser talk to Tempo
// directly, so every query is routed through the Perses datasource proxy and
// from there through the Tempo service-access boundary.
type PersesTempoPluginSpec struct {
	Proxy *PersesTempoHTTPProxy `json:"proxy,omitempty"`
	// DirectURL is the direct datasource URL. BaseHarbor never sets it: leaving
	// it empty is what keeps every query inside the Perses proxy and the
	// BaseHarbor service-access boundary.
	DirectURL string `json:"directUrl,omitempty"`
}

// PersesTempoHTTPProxy is the Perses HTTP proxy envelope for a datasource:
// the generic proxy.Proxy[http.Config] shape the pinned schema module
// declares — kind plus the HTTP config with the datasource url.
type PersesTempoHTTPProxy struct {
	Kind string                  `json:"kind"`
	Spec PersesTempoHTTPProxyCfg `json:"spec"`
}

// PersesTempoHTTPProxyCfg is the HTTP proxy config the upstream
// http.Config declares; BaseHarbor emits the datasource url only.
type PersesTempoHTTPProxyCfg struct {
	URL string `json:"url"`
}

// PersesTempoTraceQuery is the native Perses panel query of wrapper kind
// "TraceQuery" whose spec.plugin is the pinned TempoTraceQuery plugin bound to
// this datasource: the exact shape the v0.55.1 query model and
// dev/data/9-dashboard.json use (kind + spec; spec carries the plugin).
type PersesTempoTraceQuery struct {
	Kind string                     `json:"kind"`
	Spec PersesTempoTraceQueryOuter `json:"spec"`
}

// PersesTempoTraceQueryOuter is the spec section of the TraceQuery wrapper: it
// carries the plugin envelope, never the query fields directly.
type PersesTempoTraceQueryOuter struct {
	Plugin PersesTempoTraceQueryPlugin `json:"plugin"`
}

// PersesTempoTraceQueryPlugin is the plugin.Plugin envelope of the trace query:
// the pinned TempoTraceQuery plugin kind plus its spec.
type PersesTempoTraceQueryPlugin struct {
	Kind string                    `json:"kind"`
	Spec PersesTempoTraceQuerySpec `json:"spec"`
}

// PersesTempoTraceQuerySpec is the TempoTraceQuery plugin spec: the datasource
// selector plus the TraceQL expression (the shape the tempo plugin's
// query.cue schema declares).
type PersesTempoTraceQuerySpec struct {
	Datasource PersesTempoDatasourceSelector `json:"datasource"`
	Query      string                        `json:"query"`
}

// PersesTempoDatasourceSelector is the Perses datasource selector: the plugin
// kind plus the datasource name.
type PersesTempoDatasourceSelector struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesTempoDatasourceResult is the adapter's fail-closed verdict for the
// selected Tempo datasource.
type PersesTempoDatasourceResult struct {
	// Resource is the native Perses datasource resource.
	Resource PersesTempoResource
	// TraceQuery is the native TempoTraceQuery spec that selects Resource.
	TraceQuery PersesTempoTraceQuery
	// Owner is the real observability data owner this datasource visualises:
	// the Tempo provider kind, as an owner reference. It is never serialized:
	// Perses declares no application capability of its own.
	Owner string
	// Reachability is exactly one of PersesTempoReachabilityUnverified (no live
	// query observed) or PersesTempoReachabilityVerified (a real TraceQL query
	// through the authorized path answered). The probe-less build never yields
	// "verified".
	Reachability string
}

// Perses Tempo datasource reachability states.
const (
	// PersesTempoReachabilityUnverified is the build-only state: the resource is
	// defined, validated and authorized, but no live trace query was observed.
	PersesTempoReachabilityUnverified = "unverified"
	// PersesTempoReachabilityVerified requires a real, successful TraceQL answer
	// over the authorized Tempo service-access path.
	PersesTempoReachabilityVerified = "verified"
)

// tempoTraceQueryProbe is the injectable trace-query path used by the datasource
// health contract. Production uses the Tempo service-access client through
// queryPersesTempoTrace; tests inject a scripted probe.
type tempoTraceQueryProbe interface {
	queryTrace(ctx context.Context, proxyURL, traceQL string) (int, []string, error)
}

// unavailableTempoProbe performs no live observation: it never fails and never
// verifies, so the probe-less build stays reachability-neutral.
type unavailableTempoProbe struct{}

func (unavailableTempoProbe) queryTrace(context.Context, string, string) (int, []string, error) {
	return 0, nil, nil
}

// httpClientTempoProbe adapts the real Tempo query path to the probe. It is a
// pointer type so a nil *httpClientTempoProbe still means "no injected probe":
// the build path then issues the query itself over the authorized
// service-access client of the materialized provider.
type httpClientTempoProbe struct {
	client *http.Client
}

func (p httpClientTempoProbe) queryTrace(ctx context.Context, baseURL, traceQL string) (int, []string, error) {
	return queryPersesTempoTrace(ctx, p.client, baseURL+PersesTempoTraceQueryPath, traceQL)
}

// fallbackTempoProbe lets a scripted probe fall back to the real, authorized
// service-access query whenever it observed nothing: the URL and identity of
// the materialized provider keep winning over a test's own listener, so an
// injected probe can never paper over the production path. The build path
// hands the probe the datasource proxy URL (without the query path suffix);
// ownsTarget tells the fallback whether it must append it itself.
type fallbackTempoProbe struct {
	scripted tempoTraceQueryProbe
	fallback *http.Client
	// ownsTarget marks that the build path handed the probe the datasource
	// proxy URL (without the query-path suffix), so the fallback must append
	// it itself.
	ownsTarget bool
	// verifyAgainstReal forces the real authorized query even when the script
	// already saw traces: "verified" then requires the production path to
	// answer too.
	verifyAgainstReal bool
}

func (p fallbackTempoProbe) queryTrace(ctx context.Context, baseURL, traceQL string) (int, []string, error) {
	status, ids, err := p.scripted.queryTrace(ctx, baseURL, traceQL)
	if err != nil {
		return status, ids, err
	}
	if len(ids) > 0 && !p.verifyAgainstReal {
		return status, ids, err
	}
	target := baseURL
	if p.ownsTarget {
		target = baseURL + PersesTempoTraceQueryPath
	}
	realStatus, realIDs, realErr := queryPersesTempoTrace(ctx, p.fallback, target, traceQL)
	if realErr != nil {
		return realStatus, nil, realErr
	}
	if len(realIDs) == 0 {
		// The authorized production query answered cleanly but empty. A
		// scripted answer cannot revive it: the observation stays unverified.
		return realStatus, nil, nil
	}
	if len(ids) == 0 {
		ids = realIDs
	}
	return realStatus, ids, nil
}

// BuildPersesTempoDatasource builds the native Perses Tempo datasource for the
// selected, pinned surface. It resolves the authorized service-access client of
// the materialized provider (an unresolvable identity fails closed) but
// performs no live observation, so its reachability is always "unverified";
// only BuildPersesTempoDatasourceWithProbe and
// VerifyPersesTempoDatasourceHealth can produce "verified".
func BuildPersesTempoDatasource(m application.Manifest) (PersesTempoDatasourceResult, error) {
	return buildPersesTempoDatasourceWithProbe(m, capability.PersesDefaultRelease, unavailableTempoProbe{})
}

// BuildPersesTempoDatasourceWithProbe builds the datasource like
// BuildPersesTempoDatasource and additionally runs the real TraceQL query over
// the authorized Tempo service-access path, so a verified answer is produced
// only by an actual query.
//
// The build path resolves the production client BEFORE consulting the probe:
// without any observation target (no provider material, no endpoint, denied
// authorization, non-native shape) it fails closed without calling the probe.
// A probe wrapped in fallbackTempoProbe can fall back to the production client;
// a bare scripted probe decides the observation alone. A caller that wants a
// pure no-observation build uses BuildPersesTempoDatasource.
func BuildPersesTempoDatasourceWithProbe(m application.Manifest, probe tempoTraceQueryProbe) (PersesTempoDatasourceResult, error) {
	if probe == nil {
		probe = unavailableTempoProbe{}
	}
	return buildPersesTempoDatasourceWithProbe(m, capability.PersesDefaultRelease, probe)
}

// buildVerifiedPersesTempoDatasource is the verifying entry point used by the
// cells that must not let a script stand in for the production path: every
// observation also travels the real authorized service-access query, and a
// clean empty production answer downgrades any scripted traces to unverified.
func buildVerifiedPersesTempoDatasource(m application.Manifest, probe tempoTraceQueryProbe) (PersesTempoDatasourceResult, error) {
	_, client, err := persesTempoDatasourceSource(m)
	if err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	return buildPersesTempoDatasourceWithProbe(m, capability.PersesDefaultRelease, fallbackTempoProbe{
		scripted:          probe,
		fallback:          client,
		ownsTarget:        true,
		verifyAgainstReal: true,
	})
}

// buildPersesTempoDatasourceWithProbe is the shared implementation keyed on an
// explicit release and probe. A nil probe means the production path: the real
// TraceQL query over the authorized Tempo service-access client of the
// materialized provider. Tests inject a scripted probe.
func buildPersesTempoDatasourceWithProbe(m application.Manifest, release capability.PersesRelease, probe tempoTraceQueryProbe) (PersesTempoDatasourceResult, error) {
	// Fail-closed: without a pinned release that validates, the surface is not
	// configured, never "configured".
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	// Fail-closed on the Perses surface selection.
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesTempoDatasourceResult{}, &TempoSurfaceNotSelectedError{}
	}
	// Fail-closed on selection: Tempo is materialized only when the Traces
	// signal is selected. An unselected Traces path never yields a datasource.
	if !application.HasTraceSignal(m) {
		return PersesTempoDatasourceResult{}, &TempoNotSelectedError{}
	}
	denied, err := probeAuthorizationDenied(m)
	if err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	if denied {
		return PersesTempoDatasourceResult{}, &TempoAuthorizationError{Detail: "the traces collection deployment policy denies this application's trace sources"}
	}
	endpoint, client, err := persesTempoDatasourceSource(m)
	if err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	var traceIDs []string
	resource := PersesTempoResource{
		Kind:     PersesTempoResourceKind,
		Metadata: PersesTempoMetadata{Name: PersesTempoDatasourceName, Project: PersesTempoProject},
		Spec: PersesTempoResourceSpec{
			Default: true,
			Plugin: PersesTempoPluginRef{
				Kind: PersesTempoDatasourceKind,
				// proxy-only: no directUrl, so access stays inside the
				// Perses proxy and BaseHarbor service-access boundary.
				Spec: PersesTempoPluginSpec{Proxy: &PersesTempoHTTPProxy{Kind: PersesTempoHTTPProxyKind, Spec: PersesTempoHTTPProxyCfg{URL: endpoint}}},
			},
		},
	}
	traceQuery := PersesTempoTraceQuery{
		Kind: PersesTempoTraceQueryWrapperKind,
		Spec: PersesTempoTraceQueryOuter{
			Plugin: PersesTempoTraceQueryPlugin{
				Kind: PersesTempoTraceQueryKind,
				Spec: PersesTempoTraceQuerySpec{
					Datasource: PersesTempoDatasourceSelector{Kind: PersesTempoDatasourceKind, Name: PersesTempoDatasourceName},
					Query:      tempoProviderTraceQuery(m),
				},
			},
		},
	}
	// Fail-closed native resource validation: both emitted objects — the
	// datasource resource and the trace query — must validate against the
	// pinned upstream model witness.
	pin, err := resolvePersesTempoPin()
	if err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	if err := ValidatePersesTempoResource(pin, resource); err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	if err := ValidatePersesTempoTraceQuery(pin, traceQuery); err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	// Reachability is decided by a real, successful TraceQL query — never by a
	// pin. A caller that supplies no probe performs no observation and is
	// reported "unverified"; the verifying caller (Verify below, or
	// BuildPersesTempoDatasourceWithProbe) answers through the authorized
	// service-access client, so "verified" is only ever an observed result.
	traceIDs, err = tempoDatasourceTraceIDs(probe, client, resource.Spec.Plugin.Spec.Proxy.Spec.URL, traceQuery.Spec.Plugin.Spec.Query)
	if err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	reachability := PersesTempoReachabilityUnverified
	if len(traceIDs) > 0 {
		reachability = PersesTempoReachabilityVerified
	}
	return PersesTempoDatasourceResult{
		Resource:     resource,
		TraceQuery:   traceQuery,
		Owner:        string(capability.ProviderTempo),
		Reachability: reachability,
	}, nil
}

// tempoDatasourceTraceIDs runs one trace-query probe. A nil probe means the
// production path: the real query over the authorized service-access client of
// this provider. The explicit no-observation probe yields no traces and no
// error; every other probe answers its own query.
func tempoDatasourceTraceIDs(probe tempoTraceQueryProbe, client *http.Client, baseURL, traceQL string) ([]string, error) {
	if probe == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), tempoTraceQueryTimeout)
	defer cancel()
	_, ids, err := probe.queryTrace(ctx, baseURL, traceQL)
	return ids, err
}

// persesTempoDatasourceSource resolves the materialized, authorized Tempo
// source: its loopback management endpoint plus the service-access HTTP client.
// Absent, externally-owned, unmaterialized or denied state is an error, never a
// datasource with an empty URL.
func persesTempoDatasourceSource(m application.Manifest) (string, *http.Client, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", nil, err
	}
	return persesTempoDatasourceSourceAt(dataDir, "", m)
}

func persesTempoDatasourceSourceAt(dataDir, namespace string, m application.Manifest) (string, *http.Client, error) {
	// Fail-closed on ownership before the state lookup: an external Tempo
	// placement is outside the BaseHarbor authorization boundary, so the
	// adapter names that reason itself instead of relaying a placement error.
	if placement, err := application.ResolveProviderPlacement(m, capability.ProviderTempo); err == nil && placement.Scope == capability.ScopeExternal {
		return "", nil, &TempoSourceExternalError{}
	}
	files, placement, err := traces.ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, &TempoSourceAbsentError{}
		}
		return "", nil, fmt.Errorf("perses tempo datasource provider state: %w", err)
	}
	if placement.Scope == capability.ScopeExternal {
		return "", nil, &TempoSourceExternalError{}
	}
	// The datasource endpoint addresses the managed Tempo service-access
	// gateway by its backend-network alias (P1): routable from the network
	// namespace the pinned Perses instance runs in, never the host-published
	// loopback address a consumer in another namespace cannot reach. The
	// endpoint the runtime materialized is read and checked first — it is
	// the persisted truth of the gateway listener, and a deployment whose
	// endpoint is not served over HTTPS presents no authorized boundary at
	// all, so building fails closed instead of emitting a plaintext
	// datasource. Its published port carries onto the alias
	// (traces.tempoAccessSpec maps ${BASEHARBOR_TEMPO_PORT} to the gateway's
	// access port), and the alias is a TLS identity the same spec mints into
	// the gateway certificate, so URL host and server name agree.
	endpoint, err := traces.ProviderEndpoint(files)
	if err != nil {
		return "", nil, &TempoEndpointAbsentError{Detail: err.Error()}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", nil, &TempoEndpointAbsentError{Detail: "the materialized Tempo endpoint is not a valid URL"}
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", nil, &TempoEndpointAbsentError{Detail: "the materialized Tempo endpoint is not served over HTTPS"}
	}
	// The alias URL carries the endpoint's published port, so the port must
	// really be the managed gateway's mapping: the compose file the same
	// materialization wrote has to publish BASEHARBOR_TEMPO_PORT onto the
	// gateway's access port. A provider state whose env and compose files
	// disagree (drifted or tampered state) describes a listener the gateway
	// does not own, so the datasource would name a port nothing behind the
	// alias answers — fail closed instead of emitting a datasource that can
	// never query.
	if err := tempoProviderPublishedPort(files); err != nil {
		return "", nil, err
	}
	proxyURL, err := tempoProviderAccessURL(endpoint, m)
	if err != nil {
		return "", nil, err
	}
	client, err := tempoProviderHTTPClient(m, files)
	if err != nil {
		return "", nil, err
	}
	return proxyURL, client, nil
}

const (
	// tempoAccessGatewayServiceName is the compose service name of the
	// managed Tempo service-access gateway (traces.tempoAccessSpec): the
	// alias identity the datasource addresses and the TLS identity the
	// runtime issues for that listener.
	tempoAccessGatewayServiceName = "tempo-access"
	// tempoAccessGatewayContainerPort is the gateway's access port, fixed by
	// the same spec (serviceaccess.HTTPGatewaySpec.ContainerPort). The
	// listener is the ONLY port the enforced boundary serves: the compose
	// rendering publishes ${BASEHARBOR_TEMPO_PORT} to exactly this container
	// port, so the host-side number is a transient mapping, never a second
	// listener. The datasource — and its probes — therefore always address
	// the gateway on this port, by alias; a deployment whose runtime.env
	// port disagrees with the managed mapping is checked by
	// tempoProviderPublishedPort against the compose file itself.
	tempoAccessGatewayContainerPort = 8443
)

// tempoProviderPublishedPort proves the endpoint's host port really is the
// managed gateway's published mapping: the compose file the same
// materialization wrote publishes ${BASEHARBOR_TEMPO_PORT} to the gateway's
// access port. A provider state whose env and compose files disagree (a
// tampered or stale runtime.env, or a third-party stack that borrowed the
// provider directory) describes no listener the managed gateway owns, so the
// datasource build fails closed instead of pointing Perses at it.
func tempoProviderPublishedPort(files traces.ProviderFiles) error {
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		return &TempoEndpointAbsentError{Detail: fmt.Sprintf("read the materialized Tempo compose file: %v", err)}
	}
	if !strings.Contains(string(data), fmt.Sprintf("127.0.0.1:${%s}:%d", "BASEHARBOR_TEMPO_PORT", tempoAccessGatewayContainerPort)) {
		return &TempoEndpointAbsentError{Detail: fmt.Sprintf("the materialized Tempo compose publishes no port to the managed gateway access port %d: the datasource cannot address a listener the gateway does not own", tempoAccessGatewayContainerPort)}
	}
	return nil
}

// tempoProviderHTTPClient builds the Tempo service-access client for the
// materialized provider directory. It resolves the service-access policy and
// requires the PKI material of the provider to exist: a missing or denied
// identity never yields a permissive client. The returned client trusts only
// the runtime CA bundle and presents the application client identity.
//
// The provider publishes its query endpoint as an https loopback URL while the
// runtime server certificate carries the "localhost" name, so the client pins
// the TLS server name to the authorized identity. The transport therefore
// verifies the certificate the runtime issued instead of accepting any
// loopback listener.
func tempoProviderHTTPClient(m application.Manifest, files traces.ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(m.Environment, string(capability.ProviderTempo), serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, &TempoAuthorizationError{Detail: err.Error()}
	}
	// The tempo-access gateway is an mTLS boundary BY SPEC
	// (traces.tempoAccessSpec is RequireClient: true): an operator cannot
	// select a weaker AUTHENTICATION mode on the datasource path, because the
	// gateway this client addresses would refuse the session anyway. The mode
	// is fixed here so the policy the reader carries and the posture the
	// boundary enforces can never drift apart — and the requirement raising
	// below stays the ONLY policy mutation on this path.
	policy.Authentication = serviceaccess.AuthenticationMTLS
	// Fail-closed (P2): the tempo-access gateway terminates TLS with
	// RequireAndVerifyClientCert in EVERY environment (traces.tempoAccessSpec
	// is RequireClient: true), so this client must present an enrolled
	// identity wherever the resolved policy does not already demand one —
	// the dev default of serviceaccess.Resolve marks authentication optional,
	// but the boundary this datasource addresses does not accept anonymous
	// sessions. The requirement is RAISED to the boundary's posture, never
	// lowered from it: an environment whose policy requires mTLS keeps that
	// requirement through material loading and client construction unchanged,
	// and a missing enrolled identity fails closed here, not silently at
	// query time.
	policy.AuthenticationRequired = true
	// The datasource's TLS identity reference is the gateway material ITSELF
	// (P2): the projected material EnsureHTTPGateway reconciled for the
	// boundary this client addresses — trust bundle, server pair and, under
	// the gateway's own raised posture, the enrolled client identity. The
	// managed local PKI stores a gateway's client pair in the protected pki/
	// tree and projects it container-readable to runtime/; a reader that
	// re-resolved the provider PKI under the ENVIRONMENT's default policy
	// instead would silently drop exactly that projected pair whenever the
	// policy default marks authentication optional — the downgrade the review
	// found. reconcileGatewayAuthentication raises the posture wherever the
	// gateway spec verifies clients, so the gateway material names the
	// identity the boundary actually mounts, in every environment. An
	// external or BYOC source is untouched: its material is the operator's,
	// resolved from the policy paths (the ExistingTLSMaterial branch below).
	// Under the managed-local PKI the gateway's server certificate is minted
	// to carry the access alias (traces.tempoAccessSpec), and the client the
	// projection reference below resolves pins exactly that name: URL host
	// and trust identity agree, and a listener that cannot present the
	// managed certificate cannot satisfy the path. An external or BYOC policy
	// keeps the operator's server name — ExistingTLSMaterial resolves its
	// material from the policy paths, where that name is the reference.
	if policy.PKISource == serviceaccess.PKIManagedLocal {
		policy.ServerName = tempoProviderAccessAlias()
	}
	// The datasource's TLS identity reference is the gateway material ITSELF
	// (P2): the container-readable runtime projection EnsureHTTPGateway
	// reconciles and the compose service mounts — trust bundle, server pair
	// and, wherever the gateway posture enrolls one, the client identity.
	// Reading the protected pki/ source tree instead would resurrect a pair
	// from an older, laxer reconcile the CURRENT boundary no longer
	// references. An external or BYOC source is untouched: its material —
	// and its client pair, if the policy names one — resolves from the
	// operator's policy paths (the ExistingTLSMaterial external branch
	// ignores the directory argument). A managed-local provider whose
	// projection was never reconciled has no authorized boundary and fails
	// closed here.
	gatewayDir := filepath.Join(files.Dir, "service-access")
	material, err := serviceaccess.GatewayReaderTLSMaterial(gatewayDir, policy.ServerName, policy.AuthenticationRequired && policy.Authentication == serviceaccess.AuthenticationMTLS)
	if errors.Is(err, serviceaccess.ErrGatewayMaterialNotReconciled) && policy.PKISource == serviceaccess.PKIManagedLocal {
		// A managed-local provider whose gateway reconciled BEFORE the alias
		// identities existed (an upgraded deployment) has no projection that
		// names the datasource's URL host: reading its pki/ source tree would
		// authorize a client whose pinned name no gateway certificate
		// presents. Fail closed with the reconcile-the-deployment verdict —
		// the next apply reconciles the gateway and enrolls the identity.
		return nil, &TempoAuthorizationError{Detail: fmt.Sprintf("the tempo-access gateway at %s carries no runtime projection for the datasource identity %q: reconcile the deployment", gatewayDir, policy.ServerName)}
	}
	if err != nil {
		return nil, &TempoAuthorizationError{Detail: fmt.Sprintf("load Tempo service access identity: %v", err)}
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
	if err != nil {
		return nil, &TempoAuthorizationError{Detail: err.Error()}
	}
	return client, nil
}

// probeAuthorizationDenied checks the traces collection authorization for the
// manifest: with traces collection disabled the datasource has no authorized
// data path and must fail closed.
func probeAuthorizationDenied(m application.Manifest) (bool, error) {
	policy, err := application.TracesPolicy(m)
	if err != nil {
		return false, fmt.Errorf("perses tempo datasource traces authorization: %w", err)
	}
	if !policy.Enabled {
		return false, &TempoAuthorizationError{Detail: "trace collection is disabled by deployment policy"}
	}
	return false, nil
}

// tempoProviderAccessURL renders the datasource proxy URL from the endpoint
// the runtime materialized: the host-published loopback address is replaced by
// the managed gateway's backend-network alias (P1) and the published port is
// carried through (traces.tempoAccessSpec maps ${BASEHARBOR_TEMPO_PORT} to the
// gateway's access port, so the alias with that port reaches the same enforced
// listener from any namespace that routes the backend network). The address is
// never loopback, and never the bare "localhost" name the upstream default
// deny set refuses. An external or BYOC source owns its boundary's identity
// and names it through the service-access server-name environment (the same
// policy value the client pins); a managed-local source ignores that override,
// because the managed certificate name is BaseHarbor-minted.
func tempoProviderAccessURL(endpoint string, m application.Manifest) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", &TempoEndpointAbsentError{Detail: "the materialized Tempo endpoint is not a valid URL"}
	}
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(tempoAccessGatewayContainerPort)
	}
	host := tempoProviderAccessAlias()
	if policy, policyErr := serviceaccess.Resolve(m.Environment, string(capability.ProviderTempo), serviceaccess.AuthenticationMTLS); policyErr == nil && policy.PKISource != serviceaccess.PKIManagedLocal && strings.TrimSpace(policy.ServerName) != "" {
		host = policy.ServerName
	}
	return "https://" + host + ":" + port, nil
}

// tempoProviderAccessAlias is the managed gateway alias this datasource
// addresses: the qualified backend-network service address
// (traces.ManagedAccessAliases' second entry), the canonical form the compose
// networks register. Nothing else may name the datasource's TLS identity: the
// URL host, the minted certificate name and the client's pinned server name
// are exactly this alias (P1/P2). An operator who selects an external or BYOC
// PKI source owns the boundary's identity and names it through the
// service-access environment contract (BASEHARBOR_TEMPO_SERVICE_TLS_SERVER_NAME);
// the managed-local path ignores that override — the managed certificate is
// BaseHarbor-minted and its name is fixed by this function.
func tempoProviderAccessAlias() string {
	return traces.ManagedAccessAliases(tempoAccessGatewayServiceName)[1]
}

// tempoProviderTraceQuery is the real TraceQL expression this datasource
// probes with: every trace of the application's own service. It is a genuine
// TraceQL spanset selector, the shape the pinned TempoTraceQuery plugin emits.
func tempoProviderTraceQuery(m application.Manifest) string {
	name := strings.TrimSpace(m.Name)
	if name == "" {
		name = "baseharbor"
	}
	return fmt.Sprintf(`{ .resource["service.name"] == %q }`, name)
}
