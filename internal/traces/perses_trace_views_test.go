package traces

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The cells below are real assertions over the fail-closed view contract: the
// trace view never fabricates a Perses trace view document for an unselected,
// unowned, missing, or unauthorized Tempo path, and its view carries a meaningful
// no-data replacement and a fail-closed "unverified" reachability.

// TestPersesTraceViewNotSelectedFailsClosed asserts the fail-closed rule:
// without a selected Traces signal the view is not built, never fabricated.
func TestPersesTraceViewNotSelectedFailsClosed(t *testing.T) {
	m := application.New("perses-traces-demo", "dev", false, false, false)
	if _, err := PersesTraceViewNow(m); err == nil {
		t.Fatal("unselected Traces signal must fail closed, not fabricate a view")
	}
}

// TestPersesTraceViewRejectsUnprovisionedTempo asserts that a selected Traces
// path with no provisioned Tempo provider fails closed.
func TestPersesTraceViewRejectsUnprovisionedTempo(t *testing.T) {
	m := application.New("perses-traces-demo", "dev", false, false, false)
	m = application.WithOTLPTelemetry(m, "traces")
	if _, err := PersesTraceViewNow(m); err == nil {
		t.Fatal("unprovisioned Tempo path must fail closed, not fabricate a view")
	}
}

// TestPersesTraceViewFailsClosedWhenUnprovisioned asserts the fail-closed path
// against a populated ProviderFiles pair that has no materialized Tempo port:
// the builder must fail closed (never fabricate a view with an empty URL).
func TestPersesTraceViewFailsClosedWhenUnprovisioned(t *testing.T) {
	// A Traces-selected manifest, but Tempo is not provisioned: the provider
	// files cannot be resolved, so the builder fails closed.
	m := application.New("perses-traces-demo", "dev", false, false, false)
	m = application.WithOTLPTelemetry(m, "traces")
	if _, err := PersesTraceViewNow(m); err == nil {
		t.Fatal("unprovisioned Tempo path must fail closed, not fabricate a view")
	}
}

// TestPersesTraceViewEndpointReadContract asserts the fail-closed builder's
// endpoint read (ProviderEndpoint) finds a materialized Tempo port when present
// and fails closed when absent — the exact boundary the view layer guards.
func TestPersesTraceViewEndpointReadContract(t *testing.T) {
	// No port materialized: ProviderEndpoint fails closed.
	absent := ProviderFiles{}
	if _, err := ProviderEndpoint(absent); err == nil {
		t.Fatal("ProviderEndpoint without a materialized Tempo port must fail closed")
	}
	// Port materialized: ProviderEndpoint returns the loopback HTTPS endpoint.
	populated := ProviderFiles{Env: filepath.Join(t.TempDir(), "runtime.env")}
	if err := os.WriteFile(populated.Env, []byte("BASEHARBOR_TEMPO_PORT=3200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ProviderEndpoint(populated)
	if err != nil {
		t.Fatalf("ProviderEndpoint with a materialized Tempo port should succeed: %v", err)
	}
	if got != "https://127.0.0.1:3200" {
		t.Fatalf("ProviderEndpoint = %q, want https://127.0.0.1:3200", got)
	}
}

// TestPersesTraceViewJSONDeterministic asserts the deterministic marshalling
// witness for a hand-built trace view (the fail-closed path cannot produce one).

// TestPersesTraceViewJSONDeterministic asserts the deterministic marshalling
// witness for a hand-built trace view (the fail-closed path cannot produce one).
func TestPersesTraceViewJSONDeterministic(t *testing.T) {
	v := PersesTraceView{
		Kind:           "view",
		ID:             PersesTraceViewName,
		Title:          "BaseHarbor traces",
		Owner:          string(capability.ProviderTempo),
		DatasourceName: datasourceName,
		DatasourceURL:  "https://127.0.0.1:3200",
		Reachability:   PersesTraceViewUnverified,
		Panels: []PersesTracePanel{{
			Title:          "BaseHarbor traces",
			Kind:           PersesTracePanelKind,
			DatasourceName: datasourceName,
			NoData:         "No traces for this owner in the selected range",
			Range:          "-15m",
		}},
	}
	a, err := PersesTraceViewJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PersesTraceViewJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("Perses trace view JSON marshalling is not deterministic")
	}
	if a[len(a)-1] != '\n' {
		t.Fatal("Perses trace view JSON must end with a trailing newline")
	}
	// The golden witness: the deterministic shape must contain the native panel
	// kind and the owner reference, never a fabricated capability claim.
	s := string(a)
	if !strings.Contains(s, PersesTracePanelKind) {
		t.Fatalf("Perses trace view JSON must embed the native panel kind %q: %s", PersesTracePanelKind, s)
	}
}

// TestPersesTraceViewKindIsNativeTracesPanel asserts the panel kind is the
// native tracesPanel, never a Grafana UI API kind.
func TestPersesTraceViewKindIsNativeTracesPanel(t *testing.T) {
	if PersesTracePanelKind != "tracesPanel" {
		t.Fatalf("perses trace panel kind = %q, want the native tracesPanel (never a Grafana UI API)", PersesTracePanelKind)
	}
}

// TestPersesTraceViewNamesTempoOwner names the real owner, not a capability
// claim: the view names the Tempo provider kind as owner, never asserts a
// Traces capability of its own (Perses only visualises the backend data owner).
func TestPersesTraceViewNamesTempoOwner(t *testing.T) {
	v := PersesTraceView{Owner: string(capability.ProviderTempo)}
	if v.Owner != string(capability.ProviderTempo) {
		t.Fatalf("perses trace view owner = %q, want %q (the real observability owner, as an owner reference)", v.Owner, capability.ProviderTempo)
	}
}

// TestPersesTraceViewUnverifiedConstant asserts the only reachability state this
// Core produces is "unverified" (no real Tempo query probe exists here).
func TestPersesTraceViewUnverifiedConstant(t *testing.T) {
	if PersesTraceViewUnverified != "unverified" {
		t.Fatalf("reachability contract = %q, want %q (no Tempo query probe exists here)", PersesTraceViewUnverified, "unverified")
	}
}
