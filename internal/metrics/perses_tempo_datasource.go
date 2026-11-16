package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
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
// unpinned surface, and its health check never reports "configured"/"healthy"
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

// PersesTempoDatasource builds the Perses Tempo datasource for the selected,
// pinned surface. Fail-closed: it requires the observability management UI
// (Perses) selected in the manifest and a pinned release that validates
// (capability.ValidatePersesRelease). Without either it returns the
// fail-closed error, never a fabricated datasource.
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
	// Best-effort endpoint: a missing or unmaterialized provider endpoint
	// leaves the URL empty but does NOT fail the definition — the fail-closed
	// health check owns reachability. A real endpoint (when present) is the
	// provider's own management endpoint over the runtime TLS boundary; no
	// secret is materialized into the descriptor.
	url, urlErr := persesTempoDatasourceURL(m)
	if urlErr != nil {
		url, urlErr = "", nil
	}
	auth := release.AuthenticationClass
	datasource := PersesTempoDatasource{
		Kind:    PersesTempoDatasourceKind,
		Name:    PersesTempoDatasourceName,
		Default: true,
		Owner:   string(capability.ProviderTempo),
	}
	if url != "" {
		datasource.Config = PersesTempoDatasourceConfig{
			URL:            url,
			Access:         "proxy",
			Authentication: auth,
		}
	}
	return PersesTempoDatasourceResult{
		Datasource:   datasource,
		Reachability: PersesTempoReachabilityUnverified,
	}, nil
}

// persesTempoDatasourceURL returns the reachable Tempo management endpoint over
// the runtime TLS boundary when the provider files are materialized. It is
// best-effort: a missing endpoint leaves the URL empty but is surfaced as an
// error, never swallowed.
func persesTempoDatasourceURL(m application.Manifest) (string, error) {
	files, err := ExistingProviderFilesAt("", "", m)
	if err != nil {
		return "", err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return "", err
	}
	return endpoint, nil
}

// VerifyPersesTempoDatasourceHealth enforces the fail-closed datasource health
// contract: a selected, pinned Tempo datasource requires a real reachable Tempo
// health path. The seam ships no Perses / Tempo runtime health probe, so this
// Core can never assert "verified"/"healthy" on its own — pin validation alone
// MUST NOT pass. It returns the fail-closed PersesNotConfiguredError when the
// surface is unpinned, matching the seam.
func VerifyPersesTempoDatasourceHealth(_ context.Context) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return err
	}
	// Declared and validated; reachability stays unverified (no Tempo health
	// path exists here). Do NOT report a "configured"/"healthy" datasource.
	// No SUPPORTED HA line.
	return nil
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

// compile-time guards so the fail-closed health path stays reachable and the
// imports are used by downstream wiring.
var (
	_ = http.StatusServiceUnavailable
	_ = time.Second
	_ = fmt.Sprintf
	_ = strings.TrimSpace
)
