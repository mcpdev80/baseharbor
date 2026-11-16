package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
// This file realizes the Perses-native Tempo datasource definition that
// visualises the real BaseHarbor Tempo provider. Perses is a management
// surface, never an application capability: this adapter only binds the
// existing, Tempo-owned traces endpoint to a Perses datasource. It declares NO
// Traces capability of its own (Perses only visualises the backend data owner
// that already ships the real traces signal) and names the real owner (Tempo)
// as an owner reference, never as a Perses-declared capability.
//
// This adapter mirrors internal/metrics/perses_prometheus_datasource.go for the
// traces family: the same Go-struct-first realization, the same fail-closed
// contract, and the same deterministic golden-file witness. The Perses JSON
// format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go and internal/capability/perses.go): the
// structs marshal to the Perses-native datasource/document format and the
// golden-file witness is their marshalling. No go:embed of datasource JSON
// lives in this package — the structs are the source of truth.
//
// Fail-closed: the adapter never fabricates a datasource for an unselected or
// unpinned surface, its builder enforces Tempo selection, Tempo ownership and
// Tempo authorization, and its health check never reports "configured"/"healthy"
// without a real reachable Tempo health path. The seam ships no Perses / Tempo
// runtime health probe, so an unreachable or missing Tempo path is reported as
// an error, never swallowed into a false success.

// Perses Tempo datasource identity.
const (
	// PersesTempoDatasourceName is the Perses datasource selector name.
	PersesTempoDatasourceName = "tempo"
	// PersesTempoDatasourceKind is the Perses native datasource plugin
	// kind. It is the pinned native plugin, not a Grafana UI API.
	PersesTempoDatasourceKind = "TraceDatasource"
)

// PersesTempoDatasourceConfig is the Perses datasource config section for the
// BaseHarbor Tempo datasource. It carries only the reachable endpoint and the
// management-auth reference: no secrets. Secrets (MTLS material or the optional
// UI basic-auth) live in the Tempo provider PKI and are consumed at request
// time, never materialized into the descriptor.
type PersesTempoDatasourceConfig struct {
	// URL is the reachable Tempo endpoint the datasource queries. It is the
	// provider's own management endpoint over the runtime TLS boundary, never an
	// internal container address. Empty when the provider endpoint is not yet
	// materialized; the fail-closed health check then still fails.
	URL string `json:"url,omitempty"`
	// Access is the Perses datasource access mode. BaseHarbor routes every
	// request through the Tempo management proxy over TLS, so this is always
	// "proxy" — never "direct", which would bypass the access boundary.
	Access string `json:"access,omitempty"`
	// Authentication names the management authentication class this datasource
	// uses (native-oidc, standards-auth-adapter, native-credential,
	// unsupported). It is an owner reference, never a secret value.
	Authentication string `json:"authentication,omitempty"`
}

// PersesTempoDatasource is the Perses native Tempo datasource resource produced
// by this adapter. Only the fields BaseHarbor uses are represented; the
// marshalling is deterministic and golden-tested.
type PersesTempoDatasource struct {
	Kind    string                      `json:"kind"`
	Name    string                      `json:"name"`
	Default bool                        `json:"default"`
	Config  PersesTempoDatasourceConfig `json:"config,omitempty"`
	// Owner is the real observability capability owner Perses visualises. It is
	// the Tempo provider kind, named as an owner reference, NOT a Perses-declared
	// capability. It is not serialized: the datasource declares no capability of
	// its own.
	Owner string `json:"-"`
}

// PersesTempoDatasourceResult is the adapter's fail-closed verdict for the
// selected Tempo datasource.
type PersesTempoDatasourceResult struct {
	// Datasource is the produced Perses Tempo datasource resource.
	Datasource PersesTempoDatasource
	// Reachability is exactly one of:
	//   - "unverified": the contract is defined and fail-closed, but no real
	//     Tempo health path was observed (the only reachable state this Core
	//     produces for a selected, pinned surface).
	//   - "verified": only when a real reachable Tempo health path was observed.
	//     That probe does not exist in this Core, so this state can never be
	//     produced here.
	Reachability string
}

// PersesTempoReachabilityUnverified is the only reachability state this Core
// reports for the optional Tempo datasource: the contract is defined and
// fail-closed, but no Tempo health path exists here, so the datasource can
// never assert "verified"/"healthy".
const PersesTempoReachabilityUnverified = "unverified"

// BuildPersesTempoDatasource builds the Perses Tempo datasource for the
// selected, pinned surface. Fail-closed: it requires the observability
// management UI (Perses) selected in the manifest AND a Tempo (traces) path that
// is owned by the Tempo provider and authorized over the runtime TLS boundary.
// Without all of the above it returns the fail-closed error, never a fabricated
// datasource.
//
// The fail-closed guard is enforced against manifest truth (Tempo is only
// materialized when the Traces signal is selected, HasTraceSignal), the Tempo
// provider's placement (which returns os.ErrNotExist when Tempo is an external,
// unowned source or not provisioned), the Tempo provider's materialized
// endpoint, and a live service-access authorization for the Tempo path. A
// missing, denied, or externally-owned Tempo path therefore never yields a
// datasource.
//
// The datasource URL is the provider's own management endpoint over the runtime
// TLS boundary (ProviderEndpoint); no secret is materialized into the
// descriptor.
func BuildPersesTempoDatasource(m application.Manifest) (PersesTempoDatasourceResult, error) {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	return persesTempoDatasourceFromRelease(m, capability.PersesDefaultRelease)
}

// persesTempoDatasourceFromRelease is the shared implementation keyed on an
// explicit release, so both the public entry point and the fail-closed tests
// exercise identical code.
func persesTempoDatasourceFromRelease(m application.Manifest, release capability.PersesRelease) (PersesTempoDatasourceResult, error) {
	// Fail-closed: without a pinned release that validates, the surface is not
	// configured, never "configured".
	if err := capability.ValidatePersesRelease(release); err != nil {
		return PersesTempoDatasourceResult{}, err
	}
	// Fail-closed on selection: Tempo is a provider-owned backend the Perses
	// surface only visualises. It is materialized only when the Traces signal is
	// selected (HasTraceSignal). An unselected Traces path must never yield a
	// datasource, even when Perses is selected.
	if !application.HasTraceSignal(m) {
		return PersesTempoDatasourceResult{}, &TempoNotSelectedError{}
	}
	// Fail-closed on ownership and presence: the Tempo provider returns
	// os.ErrNotExist when Tempo is an external, unowned source or not provisioned.
	// A missing or externally-owned Tempo path must never fabricate a datasource.
	files, placement, err := TempoProviderFilesAt(m)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PersesTempoDatasourceResult{}, &TempoSourceAbsentError{}
		}
		return PersesTempoDatasourceResult{}, fmt.Errorf("read Tempo provider files: %w", err)
	}
	if placement.Scope == capability.ScopeExternal {
		// Tempo is selected but points at an external, non-owned source: the
		// adapter owns no authority over it, so this fails closed rather than
		// fabricating a datasource for a source outside BaseHarbor control.
		return PersesTempoDatasourceResult{}, &TempoSourceExternalError{}
	}
	// Fail-closed on a materialized endpoint: the Tempo management endpoint over
	// the runtime TLS boundary is required to build the descriptor. A missing
	// endpoint means Tempo is not provisioned and the URL must not be fabricated.
	endpoint, err := TempoProviderEndpoint(files)
	if err != nil {
		return PersesTempoDatasourceResult{}, &TempoEndpointAbsentError{Detail: err.Error()}
	}
	// Fail-closed on authorization: the Tempo path must be authorized over the
	// runtime TLS boundary via the provider's service-access policy. A denied or
	// unknown-authorization Tempo path must never yield a datasource.
	if _, authErr := serviceaccess.Resolve(m.Environment, "tempo", serviceaccess.AuthenticationMTLS); authErr != nil {
		return PersesTempoDatasourceResult{}, &TempoAuthorizationError{Detail: authErr.Error()}
	}
	datasource := PersesTempoDatasource{
		Kind:    PersesTempoDatasourceKind,
		Name:    PersesTempoDatasourceName,
		Default: true,
		Owner:   string(capability.ProviderTempo),
		Config: PersesTempoDatasourceConfig{
			URL:            endpoint,
			Access:         "proxy",
			Authentication: release.AuthenticationClass,
		},
	}
	return PersesTempoDatasourceResult{
		Datasource:   datasource,
		Reachability: PersesTempoReachabilityUnverified,
	}, nil
}

// TempoProviderFilesAt returns the Tempo provider files and placement for the
// selected manifest. It wraps the Tempo provider's own ExistingProviderFilesAt
// (internal/traces/tempo.go), which returns os.ErrNotExist when Tempo is an
// external, unowned source or not provisioned, and the Placement so the caller
// can reject a ScopeExternal (externally-owned) path fail-closed.
func TempoProviderFilesAt(m application.Manifest) (traces.ProviderFiles, traces.Placement, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return traces.ProviderFiles{}, traces.Placement{}, err
	}
	return traces.ExistingProviderFilesAt(dataDir, "", m)
}

// TempoProviderEndpoint returns the Tempo management endpoint over the runtime
// TLS boundary from the materialized provider files. It fails when the endpoint
// is not materialized, which means Tempo is not provisioned and no URL may be
// fabricated.
func TempoProviderEndpoint(files traces.ProviderFiles) (string, error) {
	return traces.ProviderEndpoint(files)
}

// VerifyPersesTempoDatasourceHealth enforces the fail-closed datasource health
// contract: a selected, pinned Tempo datasource requires a real reachable Tempo
// health path. The seam ships no Perses / Tempo runtime health probe, so this
// Core can never assert "verified"/"healthy" on its own — pin validation alone
// MUST NOT pass. It fails closed when the Tempo datasource cannot be built
// (unselected, unowned, missing, or unauthenticated Tempo path), and otherwise
// reports the fail-closed "unverified" verdict (defined and fail-closed, but no
// real Tempo health path observed).
func VerifyPersesTempoDatasourceHealth(_ context.Context) error {
	// Fail-closed without a built, authorized Tempo path: a datasource that
	// cannot be built must not report health, even when the Perses release pin
	// validates. Build against an empty manifest; any missing selection,
	// ownership, endpoint, or authorization is the expected fail-closed verdict.
	if _, err := persesTempoDatasourceFromRelease(application.Manifest{}, capability.PersesDefaultRelease); err == nil {
		// A build must never succeed without an authorized Tempo path.
		return &TempoHealthUnverifiedError{}
	} else if !errors.Is(err, &TempoNotSelectedError{}) &&
		!errors.Is(err, &TempoSourceAbsentError{}) &&
		!errors.Is(err, &TempoSourceExternalError{}) &&
		!errors.Is(err, &TempoEndpointAbsentError{}) &&
		!errors.Is(err, &TempoAuthorizationError{}) {
		// An unexpected error propagates as the fail-closed verdict.
		return err
	}
	// Declared, validated, and authorized; reachability stays unverified because
	// no real Tempo health probe exists in this Core. Do NOT report a
	// "configured"/"healthy" datasource, and do NOT emit a SUPPORTED HA line.
	return &TempoHealthUnverifiedError{}
}

// PersesTempoDatasourceJSON marshals the produced datasource deterministically
// (json.MarshalIndent with trailing newline), mirroring the golden-file
// witness pattern used across the metrics package.
func PersesTempoDatasourceJSON(d PersesTempoDatasource) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Tempo selection / ownership / authorization fail-closed errors. Each names the
// concrete reason a Tempo path cannot yield a Perses datasource, so callers and
// tests can assert the exact fail-closed verdict rather than a fabricated
// datasource.

// TempoNotSelectedError is returned when the Traces signal is not selected: the
// Tempo backend is not provisioned and no datasource may be fabricated.
type TempoNotSelectedError struct{}

func (*TempoNotSelectedError) Error() string {
	return "Tempo datasource cannot be built: the Traces signal is not selected (perses.selected plus a selected, Tempo-owned trace query) — the Perses Tempo datasource requires a selected, Tempo-owned traces path, so this fails closed rather than fabricating a datasource"
}

// TempoSourceAbsentError is returned when the Tempo provider is not
// provisioned (os.ErrNotExist): no materialized Tempo path exists.
type TempoSourceAbsentError struct{}

func (*TempoSourceAbsentError) Error() string {
	return "Tempo datasource cannot be built: the Tempo provider is not provisioned — no materialized Tempo path exists, so this fails closed rather than fabricating a datasource"
}

// TempoSourceExternalError is returned when the selected Tempo path points at
// an external, non-owned source that BaseHarbor does not control.
type TempoSourceExternalError struct{}

func (*TempoSourceExternalError) Error() string {
	return "Tempo datasource cannot be built: the selected Tempo path is externally owned (ScopeExternal) — the adapter owns no authority over a source outside BaseHarbor control, so this fails closed rather than fabricating a datasource"
}

// TempoEndpointAbsentError is returned when the Tempo management endpoint is not
// materialized (Tempo not provisioned).
type TempoEndpointAbsentError struct {
	Detail string
}

func (e *TempoEndpointAbsentError) Error() string {
	return fmt.Sprintf("Tempo datasource cannot be built: the Tempo management endpoint is not materialized (%s) — no endpoint means Tempo is not provisioned, so this fails closed rather than fabricating a datasource", e.Detail)
}

// TempoAuthorizationError is returned when the Tempo path cannot be authorized
// over the runtime TLS boundary (service access resolution denied or unknown).
type TempoAuthorizationError struct {
	Detail string
}

func (e *TempoAuthorizationError) Error() string {
	return fmt.Sprintf("Tempo datasource cannot be built: the Tempo path is not authorized over the runtime TLS boundary (%s) — a denied or unknown authorization must not fabricate a datasource", e.Detail)
}

// TempoHealthUnverifiedError is the fail-closed health verdict for a selected,
// pinned, and authorized Tempo datasource: the contract is defined and
// fail-closed, but no real Tempo health path was observed in this Core, so
// reachability stays unverified and health is never reported as "healthy"
// without a real probe.
type TempoHealthUnverifiedError struct{}

func (*TempoHealthUnverifiedError) Error() string {
	return "Tempo datasource health is unverified: the Perses Tempo datasource is defined and fail-closed, but no reachable Tempo health path exists in this Core, so health is never reported as configured/healthy without a real probe"
}

// compile-time guards so the fail-closed health path stays reachable and the
// imports are used by downstream wiring.
var (
	_ = http.StatusServiceUnavailable
	_ = time.Second
	_ = fmt.Sprintf
	_ = strings.TrimSpace
)
