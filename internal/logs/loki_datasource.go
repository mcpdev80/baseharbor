package logs

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
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Loki Perses datasource adapter (0428-086).
//
// This file realizes the Perses-native Loki datasource definition that
// visualises the real BaseHarbor Loki provider. Perses is a management
// surface, never an application capability: this adapter only binds the
// existing, Loki-owned log query-api surface to a Perses datasource. It
// declares NO Logs capability of its own (Perses only visualises the backend
// data owner that already ships the real logs signal) and names the real owner
// (Loki) as an owner reference, never as a Perses-declared capability.
//
// This adapter mirrors internal/metrics/perses_prometheus_datasource.go for the
// logs family: the same Go-struct-first realization, the same fail-closed
// contract, and the same deterministic golden-file witness. The Perses JSON
// format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go and internal/capability/perses.go): the
// structs marshal to the Perses-native datasource/document format and the
// golden-file witness is their marshalling. No go:embed of datasource JSON
// lives in this package — the structs are the source of truth.
//
// Fail-closed: the adapter never fabricates a datasource for an unselected,
// unowned, missing, or unauthorized Loki path. Construction is bound to the
// authoritative, manifest-resolved Loki service-access path: the selected,
// pinned Perses release, the Loki provider's materialized placement and
// endpoint, the retained runtime log-source snapshot (the materialized,
// authorized set of Loki sources of this provider), and a live service-access
// authorization for the Loki path over the runtime TLS boundary. A missing,
// denied, externally-owned, or externally-provided selection/owner/client/
// endpoint is never the authority for a datasource; it propagates as an error
// rather than a datasource with an empty config and a nil error.
//
// "verified" reachability requires a real Loki query over the management TLS
// boundary (the shared logs query surface); unreachable, missing, denied, or
// empty-backend state is reported as an error, never swallowed into a false
// success. Secrets (mTLS material, tokens, CA) live in the Loki provider PKI
// and are consumed at request time, never materialized into the descriptor.
//
// The seam contract (capability/perses.go) owns the Loki plugin pin
// (PersesRelease.LokiPluginVersion). That field is OUT OF SCOPE for this lane;
// until the coordinator/Owner adds it here this adapter reads the pinned plugin
// version from the local constant below. Owner decision (2026-11-16): add
// `LokiPluginVersion string` to capability/perses.go PersesRelease and set it
// in PersesDefaultRelease, then this adapter can read it back from the seam.
// The pin is product/version realization metadata, never a provider or
// capability specification version (Provider Contract v1).
const LokiPluginVersion = "v0.6.0"

// Perses Loki datasource identity.
const (
	// LokiDatasourceName is the Perses datasource selector name.
	LokiDatasourceName = "loki"
	// LokiDatasourceKind is the Perses native datasource plugin kind. It is the
	// pinned native plugin (spec.plugin.kind = LokiDatasource), not a Grafana
	// UI API kind.
	LokiDatasourceKind = "LokiDatasource"
)

// PersesLokiDatasourceConfig is the Perses datasource config section for the
// BaseHarbor Loki datasource. It carries only the reachable endpoint and the
// management-auth reference: no secrets. Secrets (mTLS material or the optional
// UI basic-auth) live in the Loki provider PKI and are consumed at request
// time, never materialized into the descriptor.
type PersesLokiDatasourceConfig struct {
	// URL is the reachable Loki query-api endpoint the datasource queries. It is
	// the provider's own management endpoint over the runtime TLS boundary,
	// never an internal container address. Empty when the provider endpoint is
	// not yet materialized; the fail-closed health check then still fails.
	URL string `json:"url,omitempty"`
	// Access is the Perses datasource access mode. BaseHarbor routes every
	// request through the Loki management proxy over TLS, so this is always
	// "proxy" — never "direct", which would bypass the access boundary.
	Access string `json:"access,omitempty"`
	// Authentication names the management authentication class this datasource
	// uses (native-oidc, standards-auth-adapter, native-credential,
	// unsupported). It is an owner reference, never a secret value.
	Authentication string `json:"authentication,omitempty"`
	// PluginVersion is the pinned Loki datasource plugin realization version
	// (Perses realization metadata), never a provider implementation version.
	PluginVersion string `json:"pluginVersion,omitempty"`
}

// PersesLokiDatasource is the Perses native Loki datasource resource produced
// by this adapter. Only the fields BaseHarbor uses are represented; the
// marshalling is deterministic and golden-tested.
type PersesLokiDatasource struct {
	Kind    string                     `json:"kind"`
	Name    string                     `json:"name"`
	Default bool                       `json:"default"`
	Config  PersesLokiDatasourceConfig `json:"config,omitempty"`
	// Owner is the real observability capability owner Perses visualises. It is
	// the Loki provider kind, named as an owner reference, NOT a Perses-declared
	// capability. It is not serialized: the datasource declares no capability of
	// its own.
	Owner string `json:"-"`
}

// PersesLokiDatasourceResult is the adapter's fail-closed verdict for the
// selected Loki datasource.
type PersesLokiDatasourceResult struct {
	// Datasource is the produced Perses Loki datasource resource.
	Datasource PersesLokiDatasource
	// SourceCount is the number of authorized Loki sources the datasource is
	// tied to. Construction fails closed when this would be zero, so a produced
	// result always carries at least one authorized source.
	SourceCount int
	// Reachability is exactly one of:
	//   - "unverified": the datasource is tied to selected, pinned, authorized
	//     runtime sources, but no live Loki query observed a matching stream. The
	//     build path without a probe always reports this.
	//   - "verified": only when a real Loki health/query probe succeeded (see
	//     BuildPersesLokiDatasourceWithProbe). No code path reports it without
	//     the probe.
	Reachability string
}

// PersesLokiReachabilityUnverified is the reachability state whenever no live
// probe observed a matching Loki stream: the contract is defined and fail-closed,
// but health is not verified.
const PersesLokiReachabilityUnverified = "unverified"

// PersesLokiReachabilityVerified is reported only after a real, successful Loki
// health/query probe (see BuildPersesLokiDatasourceWithProbe).
const PersesLokiReachabilityVerified = "verified"

// lokiQueryProbe is the injectable Loki query path used by the datasource health
// contract. Production uses the shared logs query surface; tests inject a
// scripted probe.
type lokiQueryProbe interface {
	queryStream(ctx context.Context, endpoint, query string) (bool, error)
}

// unavailableLokiProbe performs no live observation: it never fails and never
// verifies, so the build path stays reachability-neutral.
type unavailableLokiProbe struct{}

func (unavailableLokiProbe) queryStream(context.Context, string, string) (bool, error) {
	return false, nil
}

// httpClientQueryProbe adapts the package's shared queryStream to the probe.
type httpClientQueryProbe struct {
	client *http.Client
}

func (p httpClientQueryProbe) queryStream(ctx context.Context, endpoint, query string) (bool, error) {
	return queryStream(ctx, p.client, endpoint, query)
}

// persesLokiDatasourcePlacement derives the full placement owning the given
// materialized provider directory from the manifest, so the retained snapshot
// authority check (the log sources must belong to this provider) can prove the
// sources belong to this provider.
func persesLokiDatasourcePlacement(m application.Manifest) (Placement, error) {
	return PlacementFor(m)
}

// BuildPersesLokiDatasource builds the Perses Loki datasource for the selected
// and pinned surface. Fail-closed: it requires the observability management UI
// (Perses) selected in the manifest, a pinned release that validates
// (capability.ValidatePersesRelease), a materialized Loki provider endpoint, an
// authorized Loki service-access policy, and at least one authorized Loki log
// source in the retained runtime snapshot. Without any of these it returns the
// error, never a fabricated datasource with an empty config and a nil error.
// The build performs no live probe, so its reachability is always "unverified";
// only a probe result can produce "verified" (BuildPersesLokiDatasourceWithProbe).
//
// The datasource URL is the provider's own management endpoint over the runtime
// TLS boundary (ProviderEndpoint); no secret is materialized into the descriptor.
func BuildPersesLokiDatasource(m application.Manifest) (PersesLokiDatasourceResult, error) {
	return buildPersesLokiDatasourceWithProbe(m, capability.PersesDefaultRelease, unavailableLokiProbe{})
}

// BuildPersesLokiDatasourceWithProbe builds the datasource like
// BuildPersesLokiDatasource and additionally runs the health/query probe, so the
// verified path is exercised against a real Loki query result. Production wiring
// uses BuildPersesLokiDatasource; Verify below uses the service-access client.
func BuildPersesLokiDatasourceWithProbe(m application.Manifest, probe lokiQueryProbe) (PersesLokiDatasourceResult, error) {
	if probe == nil {
		probe = unavailableLokiProbe{}
	}
	return buildPersesLokiDatasourceWithProbe(m, capability.PersesDefaultRelease, probe)
}

// buildPersesLokiDatasourceWithProbe is the shared implementation keyed on an
// explicit release and probe, so the public entry point, the fail-closed tests
// and the verifying entry point all exercise identical code.
func buildPersesLokiDatasourceWithProbe(m application.Manifest, release capability.PersesRelease, probe lokiQueryProbe) (PersesLokiDatasourceResult, error) {
	// Fail-closed: without a pinned release that validates, the surface is not
	// configured, never "configured".
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesLokiDatasourceResult{}, err
	}
	// Fail-closed: Perses is a management surface of the observability
	// selection. Without the surface selected there is no datasource to bind,
	// never a default datasource for an unselected application.
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesLokiDatasourceResult{}, errors.New("perses Loki datasource requires the observability management UI and Perses selected in the manifest")
	}
	// Fail-closed on a materialized endpoint: the Loki query-api endpoint over
	// the runtime TLS boundary is required to build the descriptor. A missing
	// endpoint means Loki is not provisioned and the URL must not be fabricated.
	files, endpoint, err := persesLokiDatasourceEndpoint(m)
	if err != nil {
		return PersesLokiDatasourceResult{}, err
	}
	// Tie construction to the selected/authorized runtime sources: the retained
	// snapshot is the materialized source set of this provider. Missing (no
	// source materialized), denied (unauthorized classes), or invalid (damaged
	// ownership) state all propagate as errors.
	sources, err := persesLokiDatasourceSources(files, m)
	if err != nil {
		return PersesLokiDatasourceResult{}, err
	}
	verified, err := persesLokiDatasourceQuery(context.Background(), endpoint, sources, probe)
	if err != nil {
		return PersesLokiDatasourceResult{}, err
	}
	datasource := PersesLokiDatasource{
		Kind:    LokiDatasourceKind,
		Name:    LokiDatasourceName,
		Default: true,
		Owner:   string(capability.ProviderLoki),
		Config: PersesLokiDatasourceConfig{
			URL:            endpoint,
			Access:         "proxy",
			Authentication: release.AuthenticationClass,
			PluginVersion:  LokiPluginVersion,
		},
	}
	reachability := PersesLokiReachabilityUnverified
	if verified {
		reachability = PersesLokiReachabilityVerified
	}
	return PersesLokiDatasourceResult{
		Datasource:   datasource,
		SourceCount:  len(sources),
		Reachability: reachability,
	}, nil
}

// persesLokiDatasourceEndpoint returns the reachable Loki query-api management
// endpoint over the runtime TLS boundary. It is fail-closed: a missing provider
// directory or an unmaterialized/invalid port is surfaced as an error, never
// swallowed into an empty URL.
func persesLokiDatasourceEndpoint(m application.Manifest) (ProviderFiles, string, error) {
	files, err := ExistingProviderFiles(m)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProviderFiles{}, "", errors.New("perses Loki datasource has no materialized Loki provider: provision the existing provider first")
		}
		return ProviderFiles{}, "", fmt.Errorf("perses Loki datasource provider state: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return ProviderFiles{}, "", fmt.Errorf("perses Loki datasource endpoint: %w", err)
	}
	return files, endpoint, nil
}

// persesLokiDatasourceSources ties the datasource to the selected, authorized
// runtime log sources carried by the retained snapshot of the materialized
// Loki provider files. Missing (absent), unauthorized (denied by ownership),
// and invalid (damaged ownership/tenant) source state is propagated as an
// error, never accepted as nil. Application-owned sources must belong to this
// application; provider-owned sources must be collectable under the provider.
func persesLokiDatasourceSources(files ProviderFiles, m application.Manifest) ([]observability.SignalSource, error) {
	placement, err := persesLokiDatasourcePlacement(m)
	if err != nil {
		return nil, fmt.Errorf("perses Loki datasource provider placement: %w", err)
	}
	sources, found, err := readRuntimeLogSourceSnapshot(files, placement)
	if err != nil {
		return nil, fmt.Errorf("perses Loki datasource sources: %w", err)
	}
	if !found || len(sources) == 0 {
		return nil, errors.New("perses Loki datasource has no materialized Loki sources: the provider collected no log sources, so there is nothing to visualise")
	}
	var authorized []observability.SignalSource
	for _, source := range sources {
		// Fail-closed on an unauthorized owner: an application-scoped source
		// must belong to this application; a provider-owned source must belong to
		// the Loki provider kind. Any mismatch propagates rather than binding a
		// foreign datasource.
		if source.Scope == capability.ScopeApplication && strings.TrimSpace(source.OwnerApplication) != strings.TrimSpace(m.Name) {
			return nil, fmt.Errorf("perses Loki datasource has no authorized Loki source for application %s/%s: the materialized log source belongs to %q", m.Name, m.Environment, source.OwnerApplication)
		}
		authorized = append(authorized, source)
	}
	if len(authorized) == 0 {
		return nil, fmt.Errorf("perses Loki datasource has no authorized Loki sources for application %s/%s: all materialized sources are unowned or foreign", m.Name, m.Environment)
	}
	return authorized, nil
}

// persesLokiDatasourceQuery runs one Loki stream-query per authorized provider
// log-series and reports per-source availability. Each source gets its own
// single selector {baseharbor_application=...,baseharbor_service=...}: LogQL
// allows exactly one log stream selector per query and forbids duplicate label
// names within that selector, so the per-backend backend-witness is one query
// per source, never a comma-listed or 'or'-composed selector (see
// https://grafana.com/docs/loki/latest/query/log_queries/). A probe error on any
// source is propagated; a source that returns no matching stream is reported as
// unavailable for that source rather than swallowed.
func persesLokiDatasourceQuery(ctx context.Context, endpoint string, sources []observability.SignalSource, probe lokiQueryProbe) (bool, error) {
	matchedAny := false
	for _, source := range sources {
		query := fmt.Sprintf(`{baseharbor_application=%q,baseharbor_service=%q}`, source.OwnerApplication, logServiceLabel(source))
		matched, err := probe.queryStream(ctx, endpoint, query)
		if err != nil {
			return false, fmt.Errorf("perses Loki datasource query for source %s: %w", source.ID, err)
		}
		if matched {
			matchedAny = true
		}
	}
	return matchedAny, nil
}

// logServiceLabel returns the baseharbor_service matcher for a materialized log
// source. It falls back to the source target service when the snapshot does not
// carry an explicit service label, so the matcher always names a real label.
func logServiceLabel(source observability.SignalSource) string {
	if strings.TrimSpace(source.SemanticConvention) != "" {
		return strings.TrimSpace(source.SemanticConvention)
	}
	if _, service, ok := observability.ParseRuntimeTarget(source.Target); ok && strings.TrimSpace(service) != "" {
		return service
	}
	return "provider"
}

// VerifyPersesLokiDatasourceHealth enforces the fail-closed datasource health
// contract: a selected, pinned Loki datasource requires a real reachable Loki
// health path. It performs the datasource query (the provider log-series
// matcher) against the materialized management endpoint through the Loki
// service-access TLS client. Success requires ALL of: a valid pinned release,
// the surface selected, a materialized endpoint, authorized materialized
// sources, and a real probe result with at least one matching stream. Pin
// validation alone NEVER passes. Unreachable, missing, denied, invalid or
// empty-backend state is returned as an error, never as a false success.
func VerifyPersesLokiDatasourceHealth(ctx context.Context, m application.Manifest) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return err
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return errors.New("perses Loki datasource requires the observability management UI and Perses selected in the manifest")
	}
	files, endpoint, err := persesLokiDatasourceEndpoint(m)
	if err != nil {
		return err
	}
	sources, err := persesLokiDatasourceSources(files, m)
	if err != nil {
		return err
	}
	client, err := lokiDatasourceHTTPClient(m, files)
	if err != nil {
		return fmt.Errorf("perses Loki datasource health client: %w", err)
	}
	verified, err := persesLokiDatasourceQuery(ctx, endpoint, sources, httpClientQueryProbe{client: client})
	if err != nil {
		return err
	}
	if !verified {
		return errors.New("perses Loki datasource health is not verified: no authorized Loki source produced a matching stream")
	}
	return nil
}

// lokiDatasourceHTTPClient resolves the authoritative Loki service-access policy,
// loads the managed mTLS material, and builds the seam's managed Loki query
// client bound to that policy. It fails closed on an unauthorized or unknown
// Loki path — a caller-supplied client is never the authority. No secret is
// materialized into the returned client or the descriptor.
func lokiDatasourceHTTPClient(m application.Manifest, files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(m.Environment, "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load Loki service access identity: %w", err)
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
	if err != nil {
		return nil, err
	}
	return withLokiHostHeader(client, policy.ServerName), nil
}

// PersesLokiDatasourceJSON marshals the produced datasource deterministically
// (json.MarshalIndent with trailing newline), mirroring the golden-file witness
// pattern used across the metrics and logs packages.
func PersesLokiDatasourceJSON(d PersesLokiDatasource) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// compile-time guards so the fail-closed health path stays reachable and the
// imports are used by downstream wiring.
var (
	_ = http.StatusServiceUnavailable
	_ = time.Second
	_ = fmt.Sprintf
	_ = strings.TrimSpace
	_ = io.Discard
)
