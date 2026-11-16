package logs

import (
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The cells below are real assertions over the fail-closed view contract: the
// view layer never fabricates a Perses log view document for an unselected,
// unvalidated, or externally-owned Loki datasource, and its view carries a
// meaningful no-data replacement and a fail-closed "unverified" reachability.

// TestPersesLogViewNotConfiguredFailsClosed asserts the fail-closed rule:
// without a selected Loki datasource the view is not built, never fabricated.
func TestPersesLogViewNotConfiguredFailsClosed(t *testing.T) {
	if _, err := PersesLogViewNow(false, okLokiOwner()); err == nil {
		t.Fatal("unselected Loki datasource must fail closed, not fabricate a view")
	}
}

// TestPersesLogViewRejectsForeignOwner asserts a Loki view must be owned by the
// Loki backend; an unknown owner provider fails closed.
func TestPersesLogViewRejectsForeignOwner(t *testing.T) {
	if _, err := PersesLogViewNow(true, LokiDatasourceOwner{Application: "acme", Provider: capability.ProviderPrometheus, Scope: string(capability.ScopeApplication)}); err == nil {
		t.Fatal("Loki view owner with foreign provider must fail closed")
	}
}

// TestPersesLogViewRejectsUnknownScope asserts an unknown placement scope fails
// closed rather than silently allowing access.
func TestPersesLogViewRejectsUnknownScope(t *testing.T) {
	if _, err := PersesLogViewNow(true, LokiDatasourceOwner{Application: "acme", Provider: capability.ProviderLoki, Scope: "everywhere"}); err == nil {
		t.Fatal("Loki view owner with unknown scope must fail closed")
	}
}

// TestPersesLogViewBuiltWhenSelected asserts the "configured" state when Perses
// is selected and the pinned release validates: a native Perses log view with
// exactly one scoped Log panel and a meaningful no-data replacement.
func TestPersesLogViewBuiltWhenSelected(t *testing.T) {
	res, err := PersesLogViewNow(true, okLokiOwner())
	if err != nil {
		t.Fatalf("selected Loki datasource should build a view: %v", err)
	}
	v := res.View
	if v.Kind != "view" {
		t.Fatalf("Perses log view kind = %q, want %q", v.Kind, "view")
	}
	if v.Owner != string(capability.ProviderLoki) {
		t.Fatalf("Perses log view owner = %q, want %q (real owner reference)", v.Owner, capability.ProviderLoki)
	}
	if v.DatasourceName != datasourceName {
		t.Fatalf("Perses log view datasource = %q, want %q", v.DatasourceName, datasourceName)
	}
	if v.Reachability != PersesLogViewUnverified {
		t.Fatalf("Perses log view reachability = %q, want %q (no real Loki query probe here)", v.Reachability, PersesLogViewUnverified)
	}
	if len(v.Panels) != 1 {
		t.Fatalf("Perses log view panel count = %d, want exactly 1 scoped Log panel", len(v.Panels))
	}
	panel := v.Panels[0]
	if panel.Kind != PersesLogPanelKind {
		t.Fatalf("Perses log panel kind = %q, want %q (native plugin, never a Grafana UI API)", panel.Kind, PersesLogPanelKind)
	}
	if panel.DatasourceName != datasourceName {
		t.Fatalf("Perses log panel datasource = %q, want %q", panel.DatasourceName, datasourceName)
	}
	if strings.TrimSpace(panel.Query) == "" {
		t.Fatal("Perses log panel must carry a scoped LogQL matcher")
	}
	if !strings.Contains(panel.Query, "acme") {
		t.Fatalf("Perses log panel matcher must be scoped by the owning application, got %q", panel.Query)
	}
	if strings.TrimSpace(panel.NoData) == "" {
		t.Fatal("Perses log panel must carry a meaningful no-data replacement, never a bare placeholder")
	}
}

// TestPersesLogViewNoDataIsMeaningful asserts the no-data replacement is a real
// human message, not an empty or placeholder string.
func TestPersesLogViewNoDataIsMeaningful(t *testing.T) {
	res, err := PersesLogViewNow(true, okLokiOwner())
	if err != nil {
		t.Fatalf("selected Loki datasource should build a view: %v", err)
	}
	noData := res.View.Panels[0].NoData
	if !strings.Contains(strings.ToLower(noData), "no logs") {
		t.Fatalf("Perses log panel no-data = %q, want a meaningful 'no logs' message", noData)
	}
}

// TestPersesLogViewJSONDeterministic asserts the deterministic marshalling
// witness: two marshalling calls over the same view produce identical bytes.
func TestPersesLogViewJSONDeterministic(t *testing.T) {
	res, err := PersesLogViewNow(true, okLokiOwner())
	if err != nil {
		t.Fatalf("selected Loki datasource should build a view: %v", err)
	}
	a, err := PersesLogViewJSON(res.View)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PersesLogViewJSON(res.View)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("Perses log view JSON marshalling is not deterministic")
	}
	if a[len(a)-1] != '\n' {
		t.Fatal("Perses log view JSON must end with a trailing newline")
	}
}

// TestPersesLogViewFailClosedErrorNamesDetail asserts the fail-closed error
// carries a concrete reason and the view is never fabricated.
func TestPersesLogViewFailClosedErrorNamesDetail(t *testing.T) {
	_, err := PersesLogViewNow(false, okLokiOwner())
	var viewErr *PersesLogViewError
	if !errors.As(err, &viewErr) {
		t.Fatalf("expected PersesLogViewError, got %T", err)
	}
	if strings.TrimSpace(viewErr.Detail) == "" {
		t.Fatal("PersesLogViewError must name the concrete fail-closed reason")
	}
}

// TestPersesLogViewKindIsNativeLogsPanel asserts the panel kind is the native
// logsPanel, never a Grafana UI API kind.
func TestPersesLogViewKindIsNativeLogsPanel(t *testing.T) {
	if PersesLogPanelKind != "logsPanel" {
		t.Fatalf("perses log panel kind = %q, want the native logsPanel (never a Grafana UI API)", PersesLogPanelKind)
	}
}

// TestPersesLogViewNamesLokiOwner names the real owner, not a capability claim:
// the view names the Loki provider kind as owner, never asserts a Logs
// capability of its own (Perses only visualises the backend data owner).
func TestPersesLogViewNamesLokiOwner(t *testing.T) {
	res, err := PersesLogViewNow(true, okLokiOwner())
	if err != nil {
		t.Fatalf("selected Loki datasource should build a view: %v", err)
	}
	if res.View.Owner != string(capability.ProviderLoki) {
		t.Fatalf("perses log view owner = %q, want %q (the real observability owner, as an owner reference)", res.View.Owner, capability.ProviderLoki)
	}
}
