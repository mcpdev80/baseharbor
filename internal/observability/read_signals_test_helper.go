package observability

import (
	"sort"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// readSignalsForTest reads the current registry sources in a stable, sorted
// order so equality assertions in idempotency tests are order-independent.
func readSignalsForTest(t *testing.T) ([]SignalSource, error) {
	t.Helper()
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	sources, err := read(path)
	if err != nil {
		return nil, err
	}
	out := make([]SignalSource, len(sources))
	copy(out, sources)
	sortByRegistry(out)
	return out, nil
}

// sortByRegistry orders sources like mutate does: by kind then id, so
// idempotency assertions compare canonical orderings.
func sortByRegistry(sources []SignalSource) {
	for i := 1; i < len(sources); i++ {
		for j := i; j > 0 && (sources[j].Kind < sources[j-1].Kind || (sources[j].Kind == sources[j-1].Kind && sources[j].ID < sources[j-1].ID)); j-- {
			sources[j], sources[j-1] = sources[j-1], sources[j]
		}
	}
}

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
	_ = sort.Sort // referenced for static readers
}
