package observability

import "testing"

// readSignalsForTest reads the current registry sources in a stable, sorted
// order so equality assertions in idempotency tests are order-independent.
func readSignalsForTest(t *testing.T) ([]SignalSource, error) {
	t.Helper()
	sources, err := ListAll()
	if err != nil {
		return nil, err
	}
	out := make([]SignalSource, len(sources))
	copy(out, sources)
	sortByRegistry(out)
	return out, nil
}

// sortByRegistry orders sources exactly like mutate's canonical sort does:
// by kind then id, so idempotency assertions compare canonical orderings
// (card 0428-168, review P2).
func sortByRegistry(sources []SignalSource) {
	for i := 1; i < len(sources); i++ {
		for j := i; j > 0 && (sources[j].Kind < sources[j-1].Kind || (sources[j].Kind == sources[j-1].Kind && sources[j].ID < sources[j-1].ID)); j++ {
			sources[j], sources[j-1] = sources[j-1], sources[j]
		}
	}
}
