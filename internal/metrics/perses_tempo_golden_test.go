package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPersesTempoDatasourceGoldenFiles pins the marshalled native resource
// against the committed golden witness, with the materialized port normalized
// so the witness is stable.
func TestPersesTempoDatasourceGoldenFiles(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	writeTempoEndpoint(t, files, tempoTraceFixturePort)
	res, err := BuildPersesTempoDatasourceWithProbe(m, &tempoProbe{})
	if err != nil {
		t.Fatalf("authorized build must succeed: %v", err)
	}
	got, err := PersesTempoDatasourceJSON(res.Resource)
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-tempo-datasource", "tempo-datasource.json"))
	if err != nil {
		t.Fatalf("golden datasource witness missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden datasource mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
	query, err := PersesTempoTraceQueryJSON(res.TraceQuery)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	goldenQuery, err := os.ReadFile(filepath.Join("testdata", "perses-tempo-datasource", "tempo-trace-query.json"))
	if err != nil {
		t.Fatalf("golden trace-query witness missing: %v", err)
	}
	if string(query) != string(goldenQuery) {
		t.Fatalf("golden trace query mismatch:\n--- got ---\n%s\n--- want ---\n%s", query, goldenQuery)
	}
}
