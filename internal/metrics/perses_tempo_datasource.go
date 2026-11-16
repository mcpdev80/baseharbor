package metrics

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// tempoTraceQueryTimeout bounds one real trace-query probe.
const tempoTraceQueryTimeout = 10 * time.Second

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
				Spec: PersesTempoPluginSpec{Proxy: &PersesTempoHTTPProxy{Kind: PersesTempoHTTPProxyKind, Spec: PersesTempoHTTPProxyCfg{URL: persesTempoProxyURL(endpoint)}}},
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
	endpoint, err := traces.ProviderEndpoint(files)
	if err != nil {
		return "", nil, &TempoEndpointAbsentError{Detail: err.Error()}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", nil, &TempoEndpointAbsentError{Detail: "the materialized Tempo endpoint is not a valid URL"}
	}
	client, err := tempoProviderHTTPClient(m, files)
	if err != nil {
		return "", nil, err
	}
	return endpoint, client, nil
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
	// The tempo-access gateway terminates TLS with RequireAndVerifyClientCert
	// (traces.tempoAccessSpec is RequireClient: true), so the boundary demands
	// a client identity in every environment, even where the dev default marks
	// authentication optional. Where the provider PKI carries an enrolled
	// client identity, that mTLS identity is used. Where it does not — the
	// managed-local provider PKI holds only the server pair until the identity
	// pipeline has enrolled a workload — the reader keeps the trust bundle and
	// the server identity alone, which mirrors exactly the client the service
	// access layer would build for this provider. The dial is named for the
	// "localhost" identity the runtime certificate carries.
	clientPolicy := policy
	clientPolicy.AuthenticationRequired = false
	// Read the material with the requirement off: the managed-local provider
	// PKI holds the server pair until the identity pipeline has enrolled a
	// workload client identity, and this reader keeps whichever pair exists.
	material, err := serviceaccess.ExistingTLSMaterial(clientPolicy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, &TempoAuthorizationError{Detail: fmt.Sprintf("load Tempo service access identity: %v", err)}
	}
	presenting := clientPolicy
	if material.ClientCertificate != "" && material.ClientKey != "" {
		// An enrolled mTLS workload identity exists: present it.
		presenting.AuthenticationRequired = true
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, presenting)
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

// persesTempoProxyURL renders the datasource proxy URL. The service-access
// gateway publishes the Tempo query API on a loopback port whose certificate is
// issued for the "localhost" identity, so the datasource addresses that name:
// scheme and port are preserved from the materialized endpoint and the
// authorized identity is the one the runtime certificate carries.
func persesTempoProxyURL(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return strings.TrimRight(endpoint, "/")
	}
	port := parsed.Port()
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		host = "localhost"
	}
	if port == "" {
		return parsed.Scheme + "://" + host
	}
	return parsed.Scheme + "://" + host + ":" + port
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

// queryPersesTempoTrace is the actual trace-query path: a TraceQL search against
// Tempo's own /api/search endpoint over the given client. It returns the HTTP
// status plus the matched trace ids. A transport failure, a non-2xx answer or an
// undecodable body is an error; a clean empty answer is not.
func queryPersesTempoTrace(ctx context.Context, client *http.Client, endpoint, traceQL string) (int, []string, error) {
	if client == nil {
		return 0, nil, errors.New("perses tempo datasource trace query has no authorized service-access client")
	}
	if strings.TrimSpace(traceQL) == "" {
		return 0, nil, errors.New("perses tempo datasource trace query requires a non-empty TraceQL expression")
	}
	target := strings.TrimRight(endpoint, "/") + "?limit=20&format=json&q=" + url.QueryEscape(traceQL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("perses tempo datasource trace query is not reachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read tempo trace query answer: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, nil, fmt.Errorf("tempo trace query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Traces []struct {
			TraceID string `json:"traceID"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return resp.StatusCode, nil, fmt.Errorf("decode tempo trace query answer: %w", err)
	}
	ids := make([]string, 0, len(payload.Traces))
	for _, trace := range payload.Traces {
		if id := strings.TrimSpace(trace.TraceID); id != "" {
			ids = append(ids, id)
		}
	}
	return resp.StatusCode, ids, nil
}

// VerifyPersesTempoDatasourceHealth enforces the fail-closed datasource health
// contract: a selected, pinned Tempo datasource requires a REAL TraceQL query
// over the authorized Tempo service-access path. Pin validation alone NEVER
// passes. Absent, unselected, external, denied or unreachable state returns an
// error; a clean empty answer is reported as unverified, never as healthy.
func VerifyPersesTempoDatasourceHealth(ctx context.Context, m application.Manifest) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return err
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return &TempoSurfaceNotSelectedError{}
	}
	if !application.HasTraceSignal(m) {
		return &TempoNotSelectedError{}
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	baseURL, client, err := persesTempoDatasourceSourceAt(dataDir, "", m)
	if err != nil {
		return err
	}
	if denied, authErr := probeAuthorizationDenied(m); authErr != nil {
		return authErr
	} else if denied {
		return &TempoAuthorizationError{Detail: "the Tempo traces collection policy denies this application's trace sources"}
	}
	deadline, cancel := context.WithTimeout(ctx, tempoTraceQueryTimeout)
	defer cancel()
	status, ids, err := queryPersesTempoTrace(deadline, client, baseURL+PersesTempoTraceQueryPath, tempoProviderTraceQuery(m))
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return &TempoHealthUnverifiedError{Detail: fmt.Sprintf("HTTP %d answered no trace for the authorized TraceQL query", status)}
	}
	return nil
}

// PersesTempoDatasourceJSON marshals the produced native datasource resource
// deterministically (json.MarshalIndent with trailing newline), the golden-file
// witness pattern used across the metrics package.
func PersesTempoDatasourceJSON(r PersesTempoResource) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PersesTempoTraceQueryJSON marshals the produced native trace query spec
// deterministically.
func PersesTempoTraceQueryJSON(q PersesTempoTraceQuery) ([]byte, error) {
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ValidatePersesTempoResource validates the emitted datasource resource
// against the pinned upstream model witness: the REST envelope kind, the
// pinned native plugin kind, the project-scoped metadata, the proxy-only
// plugin spec, and — against the upstream source itself — the serialized
// document's field paths, which must match exactly what the pinned
// v0.55.1 model declares and must not include any apiVersion field.
func ValidatePersesTempoResource(pin PersesTempoPin, r PersesTempoResource) error {
	if r.Kind != pin.Resource.Kind {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("resource kind %q is not the pinned Perses resource kind %q (a flattened datasource document is not a Perses resource)", r.Kind, pin.Resource.Kind)}
	}
	if !containsString(pin.DatasourcePluginKinds, r.Spec.Plugin.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("datasource plugin kind %q is not a plugin kind shipped by the pinned Perses release %s", r.Spec.Plugin.Kind, pin.PersesVersion)}
	}
	if strings.TrimSpace(r.Metadata.Name) == "" {
		return &TempoResourceShapeError{Detail: "the datasource resource carries no metadata.name, so no panel can select it"}
	}
	if pin.Resource.Scope == "project" && strings.TrimSpace(r.Metadata.Project) == "" {
		return &TempoResourceShapeError{Detail: "a project-scoped Perses datasource requires metadata.project"}
	}
	if r.Spec.Plugin.Spec.DirectURL != "" {
		return &TempoResourceShapeError{Detail: "the datasource declares directUrl: BaseHarbor tempo access stays inside the Perses proxy and service-access boundary"}
	}
	if r.Spec.Plugin.Spec.Proxy == nil || strings.TrimSpace(r.Spec.Plugin.Spec.Proxy.Spec.URL) == "" {
		return &TempoResourceShapeError{Detail: "the datasource carries no HTTP proxy url, so Perses could not route the trace query"}
	}
	declared, err := upstreamDatasourceFieldPaths()
	if err != nil {
		return err
	}
	emitted, err := jsonFieldPathsOfStruct(r)
	if err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the datasource document cannot be inspected: %v", err)}
	}
	if err := emittedPathsWithin(emitted, declared, "apiVersion"); err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the emitted datasource document deviates from the pinned upstream model: %v", err)}
	}
	return nil
}

// ValidatePersesTempoTraceQuery validates the emitted trace query against the
// pinned contract witness: the panel-query wrapper kind and the TempoTraceQuery
// plugin kind the pinned release ships, a non-empty TraceQL expression, and a
// selector that names a pinned datasource plugin kind. The serialized document
// must match the field paths the pinned release's OWN native TraceQuery
// documents use (dev/data witness), not a locally asserted shape.
func ValidatePersesTempoTraceQuery(pin PersesTempoPin, q PersesTempoTraceQuery) error {
	if q.Kind != pin.TraceQueryWrapperKind {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("trace query wrapper kind %q is not the query kind the pinned release ships (%q)", q.Kind, pin.TraceQueryWrapperKind)}
	}
	if !containsString(pin.TraceQueryPluginKinds, q.Spec.Plugin.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("trace query plugin kind %q is not a query kind shipped by the pinned Perses release %s", q.Spec.Plugin.Kind, pin.PersesVersion)}
	}
	if len(q.Spec.Plugin.Spec.Query) == 0 {
		return &TempoResourceShapeError{Detail: "the pinned TempoTraceQuery contract requires a non-empty TraceQL query"}
	}
	if !containsString(pin.DatasourcePluginKinds, q.Spec.Plugin.Spec.Datasource.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the trace query selects datasource kind %q, which is not a pinned Perses datasource plugin kind", q.Spec.Plugin.Spec.Datasource.Kind)}
	}
	if strings.TrimSpace(q.Spec.Plugin.Spec.Datasource.Name) == "" {
		return &TempoResourceShapeError{Detail: "the trace query selects no datasource name"}
	}
	declared, err := upstreamTraceQueryFieldPaths()
	if err != nil {
		return err
	}
	emitted, err := jsonFieldPathsOfStruct(q)
	if err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the trace query document cannot be inspected: %v", err)}
	}
	if err := emittedPathsWithin(emitted, declared, "apiVersion"); err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the emitted trace query document deviates from the pinned upstream model: %v", err)}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// jsonFieldPathsOfStruct lists the dotted JSON paths the value actually
// serializes — the same resolution encoding/json performs, over the real
// emitted object rather than a locally declared truth.
func jsonFieldPathsOfStruct(v any) ([]string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return treeFieldPaths(doc, ""), nil
}

// treeFieldPaths flattens a decoded JSON document into sorted dotted paths.
// Arrays contribute their index-free element paths.
func treeFieldPaths(node map[string]any, prefix string) []string {
	var paths []string
	for name, value := range node {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		paths = append(paths, path)
		switch child := value.(type) {
		case map[string]any:
			paths = append(paths, treeFieldPaths(child, path)...)
		}
	}
	return paths
}

// emittedPathsWithin proves every serialized path of the emitted document is a
// path the pinned upstream model declares, and that no forbidden path (a
// version field) appears.
func emittedPathsWithin(emitted, declared []string, forbidden string) error {
	for _, path := range emitted {
		if path == forbidden || strings.HasSuffix(path, "."+forbidden) {
			return fmt.Errorf("the emitted document serializes %q, which the pinned Perses v0.55.1 model does not declare anywhere", forbidden)
		}
		if !containsPathPrefix(declared, path) {
			return fmt.Errorf("the emitted document serializes %q, which the pinned upstream model does not declare", path)
		}
	}
	return nil
}

func containsPathPrefix(paths []string, want string) bool {
	for _, path := range paths {
		if path == want || strings.HasPrefix(want, path+".") {
			return true
		}
	}
	return false
}
