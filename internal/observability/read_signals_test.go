package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestReadSignalsForTestSortsStably witnesses that the helper the migration
// idempotency assertions read through (readSignalsForTest) returns the registry
// in mutate's canonical order (kind, then id), so byte-comparison assertions
// are order-independent. It lives in a _test.go file because Go only
// discovers test functions in *_test.go files (card 0428-168, CI-surfacing
// rule AGENTS 5.3).
func TestReadSignalsForTestSortsStably(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	_ = Update(MetricsSource{ID: "z-signal", Provider: capability.ProviderTempo, Class: SourceApplicationProvider, Scope: capability.ScopeApplication, OwnerApplication: "a", Network: "net", Target: "tempo:3200", Path: "/metrics", Scheme: "http"})
	_ = Update(MetricsSource{ID: "a-signal", Provider: capability.ProviderTempo, Class: SourceApplicationProvider, Scope: capability.ScopeApplication, OwnerApplication: "b", Network: "net", Target: "tempo:3200", Path: "/metrics", Scheme: "http"})
	got, err := readSignalsForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("readSignalsForTest = %d sources, want 2", len(got))
	}
	if got[0].ID != "a-signal" || got[1].ID != "z-signal" {
		t.Fatalf("readSignalsForTest not sorted by id: %q %q", got[0].ID, got[1].ID)
	}
}
