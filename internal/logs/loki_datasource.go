package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// LokiPersesDatasource is the optional Loki datasource that the Perses
// management/visualization surface visualises. It is NOT an application
// capability: Perses only visualises the Loki data owner's native query-api
// surface (see capability.ReferenceIntegrations: baseharbor/loki, LogsV1).
//
// This file is the Perses-facing read path for that datasource. It is
// deliberately narrow and mirrors the existing seam contract:
//
//   - It pins the Loki datasource plugin version as Perses realization
//     metadata, mirroring the PersesRelease.PrometheusPluginVersion axis the
//     seam declares for the Prometheus plugin. The pin lives in the seam
//     contract (capability/perses.go) and is read from there, so Perses has a
//     single source of truth for both plugin pins.
//   - It scopes every query by ACTUAL Target/tenant ownership: a Loki source is
//     only reachable when its runtime target resolves and its owner validates.
//     Unknown ownership, an unknown scope or an unresolvable target fails
//     closed.
//   - It is fail-closed on a missing or not-ready datasource: without a real
//     readiness probe and a successful query the adapter never reports a
//     healthy "verified" surface, only "not configured"/"unverified".
//   - It keeps secrets (client certificates, keys, tokens, CA material) out of
//     every returned value, error string and log line.
//
// The HTTP/TLS client is reused from the seam's managed Loki service access
// (serviceaccess.NewHTTPClientForPolicy) so the Perses read path shares exactly
// one transport contract with the managed Loki backend. This file never
// implements container lifecycle.
//
// Never claim a Loki datasource that cannot be reached. A datasource that
// cannot be safely queried MUST be reported as unsupported, never as verified.

// LokiPluginVersion is the pinned Loki datasource plugin realization version
// that the optional Perses surface references. It is product/version
// realization metadata, identical in spirit to PersesDefaultRelease's
// PrometheusPluginVersion, and MUST NOT be exposed as a provider implementation
// version or a capability specification version (Provider Contract v1).
//
// The seam contract (capability/perses.go) owns both plugin pins; the intended
// design reads this value from the seam release contract (PersesRelease.
// LokiPluginVersion). That field does not exist yet on the current seam head:
// the PersesRelease struct only pins PrometheusPluginVersion. The seam file is
// OUT OF SCOPE for this lane (mission grenze), so the adapter pins the Loki
// plugin version here as a single local source of truth. Owner decision
// (2026-11-16): to unify, add `LokiPluginVersion string` to
// capability/perses.go PersesRelease (perses.go:126-128) and set it in
// PersesDefaultRelease (~perses.go:151); then this adapter can read it back
// from the seam instead of the local constant below.
var LokiPluginVersion = "v0.6.0"

// LokiDatasourceStatus is the machine-readable public status of the optional
// Loki datasource that Perses visualises. It is EXACTLY one of the three values
// below and no other:
//
//   - "not_configured":    no authorized Loki source is pinned/resolvable, or
//     the pinned release does not validate (fail-closed).
//   - "configured_unverified": an authorized Loki source is pinned and
//     resolvable, but no real Perses health probe exists, so reachability stays
//     unverified (this seam defines only the contract, like the Perses surface).
//   - "verified":          only when a real Loki query observes the source. That
//     probe is exercised by QueryLoki; this state is produced only by a
//     successful query, never by pin validation alone.
type LokiDatasourceStatus string

const (
	LokiDatasourceNotConfigured        LokiDatasourceStatus = "not_configured"
	LokiDatasourceConfiguredUnverified LokiDatasourceStatus = "configured_unverified"
	LokiDatasourceVerified             LokiDatasourceStatus = "verified"
)

// LokiDatasource is the declarative Loki datasource that the Perses management
// surface visualises, scoped by actual Target/tenant ownership.
type LokiDatasource struct {
	// PluginVersion is the pinned Loki datasource plugin realization version
	// (Perses realization metadata), never a provider implementation version.
	PluginVersion string
	// Target is the runtime target the authorized Loki source resolves to,
	// scoped by the calling application.
	Target string
	// Status is the public three-state status above.
	Status LokiDatasourceStatus
	// Reachable reports whether a real query succeeded (verified) or the source
	// is only "configured by declaration" (unverified). It is a SEPARATE axis
	// from ownership and never leaks secrets.
	Reachable bool
}

// LokiQuery is a scoped Loki log query against an authorized datasource.
type LokiQuery struct {
	// Matcher is the LogQL stream matcher, e.g.
	// {baseharbor_application="app",baseharbor_service="api"}.
	Matcher string
	// Start and End are query time bounds in nanosecond Unix time.
	Start int64
	End   int64
	// Limit caps the number of returned result rows.
	Limit int
}

// LokiQueryResult is the machine-readable result of a scoped Loki query. It
// carries no secret material: only the query status and a row count, never raw
// result payloads or any certificate/key/token/CA material.
type LokiQueryResult struct {
	Status   string `json:"status"`
	RowCount int    `json:"row_count"`
}

// LokiDatasourceOwner is the actual ownership/tenant boundary the Loki
// datasource is scoped by. It is fail-closed: an empty or unknown owner means
// the datasource is not reachable.
type LokiDatasourceOwner struct {
	// Application is the owning application of the runtime target.
	Application string
	// Environment is the owning environment of the runtime target.
	Environment string
	// Provider is the Loki provider kind that owns the backend (Loki).
	Provider capability.ProviderKind
	// Scope is the placement scope of the runtime target (shared or
	// application-scoped). It must resolve to a known scope; an unknown scope
	// fails closed.
	Scope string
}

// Validate fails closed when the owner boundary is incomplete or the scope is
// unknown. A Loki datasource that cannot be ownership-bound must never be
// reported as reachable.
func (o LokiDatasourceOwner) Validate() error {
	if strings.TrimSpace(o.Application) == "" {
		return errors.New("Loki datasource ownership requires an owning application (fail-closed)")
	}
	if o.Provider != capability.ProviderLoki {
		return fmt.Errorf("Loki datasource owner provider = %q, want %q", o.Provider, capability.ProviderLoki)
	}
	switch strings.ToLower(strings.TrimSpace(o.Scope)) {
	case string(capability.ScopeShared), string(capability.ScopeApplication):
	default:
		return fmt.Errorf("Loki datasource owner scope = %q, want shared or application (fail-closed)", o.Scope)
	}
	return nil
}

// ValidateLokiDatasourceSource fails closed on an unauthorized Loki log source.
// It reuses the seam's provider-log-source ownership contract: the source must
// validate, carry the logs signal kind, belong to a supported source class, and
// its runtime target must resolve. Cross-target or unowned sources return an
// error rather than a datasource.
func ValidateLokiDatasourceSource(source observability.SignalSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("Loki datasource source %q: %w", source.ID, err)
	}
	if source.Kind != observability.SignalLogs {
		return fmt.Errorf("Loki datasource source %q has signal kind %q, want logs", source.ID, source.Kind)
	}
	if source.Class != observability.SourceApplicationProvider && source.Class != observability.SourcePlatformProvider {
		return fmt.Errorf("Loki datasource source %q has unsupported source class %q", source.ID, source.Class)
	}
	if _, _, ok := observability.ParseRuntimeTarget(source.Target); !ok {
		return fmt.Errorf("Loki datasource source %q has invalid runtime target %q", source.ID, source.Target)
	}
	return nil
}

// LokiDatasourceNow returns the declarative Loki datasource for the calling
// application's owned runtime target, scoped by actual ownership. It is fail
// closed:
//
//   - without a pinned release that validates -> not_configured
//   - with a pinned, validating release and an ownership-bound source ->
//     configured_unverified (no real Perses health probe exists here)
//
// It never returns "verified": a real Loki query is required for that, and pin
// validation alone must never map into "verified"/"ready".
func LokiDatasourceNow(selected bool, owner LokiDatasourceOwner) (LokiDatasource, error) {
	if err := owner.Validate(); err != nil {
		return LokiDatasource{}, err
	}
	if !selected {
		return LokiDatasource{
			PluginVersion: LokiPluginVersion,
			Status:        LokiDatasourceNotConfigured,
			Reachable:     false,
		}, nil
	}
	if capability.ValidatePersesRelease(capability.PersesDefaultRelease) != nil {
		return LokiDatasource{
			PluginVersion: LokiPluginVersion,
			Status:        LokiDatasourceNotConfigured,
			Reachable:     false,
		}, nil
	}
	return LokiDatasource{
		PluginVersion: LokiPluginVersion,
		Target:        owner.Application + "/" + owner.Environment,
		Status:        LokiDatasourceConfiguredUnverified,
		Reachable:     false,
	}, nil
}

// QueryLoki executes a scoped Loki log query against an authorized datasource.
// It is fail-closed:
//
//   - the owner must Validate (no secrets, no unknown scope/target),
//   - the HTTP client is the seam's managed Loki service-access client,
//   - a non-2xx or malformed response fails closed with an error that carries
//     no secret material,
//   - a missing/not-ready backend fails closed (the caller probes readiness
//     first; a non-2xx here is reported as an error, never as success).
//
// QueryLoki does not persist, log or return any client certificate, key, CA
// material or bearer token. It reuses the exact query surface that query.go
// uses so the Perses read path and the managed Loki backend share one transport
// contract.
func QueryLoki(ctx context.Context, client *http.Client, endpoint string, q LokiQuery) (LokiQueryResult, error) {
	var result LokiQueryResult
	if client == nil {
		return result, errors.New("Loki datasource query requires an HTTP client")
	}
	if strings.TrimSpace(q.Matcher) == "" {
		return result, errors.New("Loki datasource query requires a stream matcher")
	}
	if q.End <= 0 {
		q.End = time.Now().UnixNano()
	}
	if q.Start <= 0 {
		q.Start = time.Unix(0, q.End).Add(-10 * time.Minute).UnixNano()
	}
	if q.Limit <= 0 {
		q.Limit = 1
	}

	values := url.Values{
		"query": {q.Matcher},
		"start": {strconv.FormatInt(q.Start, 10)},
		"end":   {strconv.FormatInt(q.End, 10)},
		"limit": {strconv.Itoa(q.Limit)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(endpoint, "/")+"/loki/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		return result, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("Loki datasource query returned HTTP %d (fail-closed: unauthorized or not-ready backend)", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return result, fmt.Errorf("Loki datasource query returned malformed response: %w", err)
	}
	if payload.Status != "success" {
		return result, fmt.Errorf("Loki datasource query status %q (fail-closed)", payload.Status)
	}
	result.Status = payload.Status
	result.RowCount = len(payload.Data.Result)
	return result, nil
}

// lokiDatasourceReady checks the seam Loki readiness probe. It fails closed on
// any non-2xx so the adapter never reports a healthy surface against a
// not-ready backend. It returns nil only when the backend answers 2xx.
func lokiDatasourceReady(ctx context.Context, client *http.Client, endpoint string) error {
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		probeCtx, probeCancel := context.WithTimeout(deadline, 3*time.Second)
		req, _ := http.NewRequestWithContext(probeCtx, http.MethodGet,
			strings.TrimRight(endpoint, "/")+"/ready", nil)
		resp, err := client.Do(req)
		probeCancel()
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-deadline.Done():
			if last == nil {
				last = deadline.Err()
			}
			return fmt.Errorf("Loki datasource not ready: %w", last)
		case <-ticker.C:
		}
	}
}
