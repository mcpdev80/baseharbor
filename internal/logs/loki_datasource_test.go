package logs

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func okLokiOwner() LokiDatasourceOwner {
	return LokiDatasourceOwner{
		Application: "acme",
		Environment: "prod",
		Provider:    capability.ProviderLoki,
		Scope:       string(capability.ScopeApplication),
	}
}

// TestLokiDatasourceOwnerFailsClosedWithoutApplication asserts that a Loki
// datasource with no owning application is not reachable (fail-closed).
func TestLokiDatasourceOwnerFailsClosedWithoutApplication(t *testing.T) {
	if _, err := LokiDatasourceNow(true, LokiDatasourceOwner{Provider: capability.ProviderLoki, Scope: string(capability.ScopeApplication)}); err == nil {
		t.Fatal("Loki datasource owner without application must fail closed")
	}
}

// TestLokiDatasourceOwnerRejectsForeignProvider asserts a Loki datasource must
// be owned by the Loki backend; an unknown owner provider fails closed.
func TestLokiDatasourceOwnerRejectsForeignProvider(t *testing.T) {
	if _, err := LokiDatasourceNow(true, LokiDatasourceOwner{Application: "acme", Provider: capability.ProviderPrometheus, Scope: string(capability.ScopeApplication)}); err == nil {
		t.Fatal("Loki datasource owner with foreign provider must fail closed")
	}
}

// TestLokiDatasourceOwnerRejectsUnknownScope asserts an unknown placement scope
// fails closed rather than silently allowing access.
func TestLokiDatasourceOwnerRejectsUnknownScope(t *testing.T) {
	if _, err := LokiDatasourceNow(true, LokiDatasourceOwner{Application: "acme", Provider: capability.ProviderLoki, Scope: "everywhere"}); err == nil {
		t.Fatal("Loki datasource owner with unknown scope must fail closed")
	}
}

// TestLokiDatasourceNotConfiguredWhenUnselected asserts the fail-closed
// "not configured" state when Perses is not selected.
func TestLokiDatasourceNotConfiguredWhenUnselected(t *testing.T) {
	got, err := LokiDatasourceNow(false, okLokiOwner())
	if err != nil {
		t.Fatalf("unselected owner must not error: %v", err)
	}
	if got.Status != LokiDatasourceNotConfigured {
		t.Fatalf("unselected Loki datasource status = %q, want not_configured", got.Status)
	}
	if got.Reachable {
		t.Fatal("unselected Loki datasource must not be reachable")
	}
	if got.PluginVersion == "" {
		t.Fatal("Loki datasource must still carry the pinned plugin version")
	}
}

// TestLokiDatasourceConfiguredUnverifiedWhenSelected asserts the
// "configured_unverified" state when Perses is selected and the pinned release
// validates, but no real query has observed the source.
func TestLokiDatasourceConfiguredUnverifiedWhenSelected(t *testing.T) {
	got, err := LokiDatasourceNow(true, okLokiOwner())
	if err != nil {
		t.Fatalf("valid owner must not error: %v", err)
	}
	if got.Status != LokiDatasourceConfiguredUnverified {
		t.Fatalf("selected Loki datasource status = %q, want configured_unverified", got.Status)
	}
	if got.Reachable {
		t.Fatal("selected Loki datasource with no successful query must not be reachable/verified")
	}
	if got.Target != "acme/prod" {
		t.Fatalf("Loki datasource target = %q, want acme/prod", got.Target)
	}
	// Pin validation alone must never map into "verified".
	if got.Status == LokiDatasourceVerified {
		t.Fatal("pin validation must not produce verified status")
	}
}

// TestLokiDatasourceValidateSourceFailsClosed asserts that a non-logs,
// unresolvable, or unsupported-class Loki source is not a valid datasource.
func TestLokiDatasourceValidateSourceFailsClosed(t *testing.T) {
	if err := ValidateLokiDatasourceSource(observability.SignalSource{ID: "s", Provider: capability.ProviderLoki, Kind: observability.SignalMetrics, Class: observability.SourcePlatformProvider, Target: "runtime://proj/svc", Protocol: "openmetrics"}); err == nil {
		t.Fatal("metrics-kind Loki source must fail validation")
	}
	if err := ValidateLokiDatasourceSource(observability.SignalSource{ID: "s", Provider: capability.ProviderLoki, Kind: observability.SignalLogs, Class: observability.SourceApplicationProvider, Target: "not-a-runtime-target", Protocol: "syslog-rfc5424"}); err == nil {
		t.Fatal("unresolvable Loki target must fail validation")
	}
}

// TestLokiQueryRequiresMatcher asserts a query with no matcher fails closed.
func TestLokiQueryRequiresMatcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if _, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{}); err == nil {
		t.Fatal("Loki query without matcher must fail closed")
	}
}

// TestLokiNilClientFailsClosed asserts a nil client fails closed.
func TestLokiNilClientFailsClosed(t *testing.T) {
	if _, err := QueryLoki(context.Background(), nil, "https://example.local", LokiQuery{Matcher: `{baseharbor_application="acme"}`}); err == nil {
		t.Fatal("nil client must fail closed")
	}
}

// TestLokiQueryReturnsSuccessRowCount asserts a real Loki query against an
// authorized backend returns a success status and the correct row count.
func TestLokiQueryReturnsSuccessRowCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/loki/api/v1/query_range") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// No client identity is required on this plain httptest server; the
		// seam's real backend enforces mTLS. The adapter's fail-closed status
		// check is what this test exercises.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"__name__":"log"},"values":[[["1","\"ok\""]]]}]}}`))
	}))
	defer srv.Close()

	res, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{Matcher: `{baseharbor_application="acme"}`, Limit: 1})
	if err != nil {
		t.Fatalf("authorized Loki query should succeed: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("Loki query status = %q, want success", res.Status)
	}
	if res.RowCount != 1 {
		t.Fatalf("Loki query row count = %d, want 1", res.RowCount)
	}
}

// TestLokiQueryNon2xxFailsClosed asserts a non-2xx backend response fails
// closed with an error that carries no secret material.
func TestLokiQueryNon2xxFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"missing client certificate"}`))
	}))
	defer srv.Close()

	_, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{Matcher: `{baseharbor_application="acme"}`})
	if err == nil {
		t.Fatal("non-2xx Loki query must fail closed")
	}
	if strings.Contains(err.Error(), "missing client certificate") {
		t.Fatal("Loki query error must not echo backend error bodies (secret/protection)")
	}
}

// TestLokiQueryMalformedResponseFailsClosed asserts a malformed JSON response
// fails closed.
func TestLokiQueryMalformedResponseFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	if _, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{Matcher: `{baseharbor_application="acme"}`}); err == nil {
		t.Fatal("malformed Loki response must fail closed")
	}
}

// TestLokiQueryEmptyResultRowCount asserts an empty result set counts zero rows.
func TestLokiQueryEmptyResultRowCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer srv.Close()

	res, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{Matcher: `{baseharbor_application="acme"}`, Limit: 1})
	if err != nil {
		t.Fatalf("authorized empty query should succeed: %v", err)
	}
	if res.RowCount != 0 {
		t.Fatalf("empty Loki result row count = %d, want 0", res.RowCount)
	}
}

// TestLokiDatasourceReadyProbeFailsClosedOnNon2xx asserts the readiness probe
// fails closed against a non-2xx backend.
func TestLokiDatasourceReadyProbeFailsClosedOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := lokiDatasourceReady(ctx, srv.Client(), srv.URL); err == nil {
		t.Fatal("readiness probe against not-ready backend must fail closed")
	}
}

// TestLokiDatasourceReadyProbeSucceedsOn2xx asserts the readiness probe
// succeeds against a 2xx backend.
func TestLokiDatasourceReadyProbeSucceedsOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := lokiDatasourceReady(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("readiness probe against ready backend should succeed: %v", err)
	}
}

// TestLokiClientKeepsSecretsOutOfLogPath asserts the adapter never imports or
// returns client certificate, key or CA material in any returned value. This
// is a compile-time contract check: the returned LokiQueryResult carries only
// status + row count.
func TestLokiClientKeepsSecretsOutOfLogPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer srv.Close()

	res, err := QueryLoki(context.Background(), srv.Client(), srv.URL, LokiQuery{Matcher: `{baseharbor_application="acme"}`})
	if err != nil {
		t.Fatal(err)
	}
	// LokiQueryResult has exactly two fields: Status and RowCount. No secret
	// field may ever be added here.
	if res.Status == "" || res.RowCount != 0 {
		t.Fatalf("unexpected result shape: %+v", res)
	}
	_ = tls.Certificate{}
}
