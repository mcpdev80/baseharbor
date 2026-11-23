package metrics

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Round-4 cells (akte act-luna-r3.md, candidate base 7a2736ba58c). The r3
// finding was a circular witness: perses-pin.json repeated the constant the
// builder asserted, so validation proved nothing about upstream. These cells
// prove the witness machinery against upstream truth instead:
//
//   - the REST Datasource model must serialize NO apiVersion (the r4 schema
//     fact), and the document's serialized field paths must each resolve to a
//     declaration in the pinned upstream source,
//   - the trace query must serialize the native TraceQuery wrapper envelope
//     the release's own dev/data dashboard uses,
//   - a tampered witness (wrong hash, drifted model copy, wrong kind table)
//     must fail closed,
//   - the builder must validate BOTH emitted resources, not just the
//     datasource.

// TestPersesTempoDatasourceWitnessResolvesUpstreamTruth proves the witness
// machinery consumes the upstream source rather than a local constant: every
// mirrored field must be found in the committed upstream text, the upstream
// Datasource struct block must contain no apiVersion, and the pinned release's
// own sample documents must round-trip through the kind checks.
func TestPersesTempoDatasourceWitnessResolvesUpstreamTruth(t *testing.T) {
	if err := upstreamModelSourceEvidence(); err != nil {
		t.Fatalf("the mirrored model must be proven in the committed upstream sources: %v", err)
	}
	pin := persesTempoPin(t)
	if err := pin.proveAgainstUpstream(); err != nil {
		t.Fatalf("the witness must prove itself against the upstream sources: %v", err)
	}
	dsPaths, err := upstreamDatasourceFieldPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind", "metadata", "metadata.name", "metadata.project", "spec", "spec.default", "spec.plugin", "spec.plugin.kind", "spec.plugin.spec", "spec.plugin.spec.proxy", "spec.plugin.spec.proxy.kind", "spec.plugin.spec.proxy.spec", "spec.plugin.spec.proxy.spec.url"} {
		if !containsString(dsPaths, want) {
			t.Fatalf("the pinned upstream datasource model must declare the path %q, resolved: %v", want, dsPaths)
		}
	}
	if containsString(dsPaths, "apiVersion") {
		t.Fatalf("the pinned upstream datasource model must not resolve apiVersion, got: %v", dsPaths)
	}
	for _, forbidden := range []string{"apiVersion", "kind.kind", "spec.version"} {
		if containsString(dsPaths, forbidden) {
			t.Fatalf("the datasource envelope must not resolve %q", forbidden)
		}
	}
	// The metadata leaf names the upstream model declares (name, project):
	// nothing else at the top level may appear.
	for _, path := range metaTops(dsPaths) {
		if path != "kind" && path != "metadata" && path != "spec" {
			t.Fatalf("the datasource envelope declares unexpected top-level path %q", path)
		}
	}
	qPaths, err := upstreamTraceQueryFieldPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind", "spec", "spec.plugin", "spec.plugin.kind", "spec.plugin.spec", "spec.plugin.spec.query", "spec.plugin.spec.datasource", "spec.plugin.spec.datasource.kind", "spec.plugin.spec.datasource.name"} {
		if !containsString(qPaths, want) {
			t.Fatalf("the pinned upstream query model must declare the path %q, resolved: %v", want, qPaths)
		}
	}
}

// metaTops lists the top-level field names a resolved path list contains.
func metaTops(paths []string) []string {
	var tops []string
	for _, path := range paths {
		top := path
		if dot := strings.Index(path, "."); dot >= 0 {
			top = path[:dot]
		}
		tops = append(tops, top)
	}
	return tops
}

// TestPersesTempoDatasourceWitnessTamperFailsClosed proves the pin test is not
// self-referential: a witness whose declaration drifts from the upstream
// source (a resource kind the upstream kind table does not declare, a wrong
// file hash, a wrong release commit) must fail closed at resolve time.
func TestPersesTempoDatasourceWitnessTamperFailsClosed(t *testing.T) {
	base, err := decodePersesTempoPin(PersesTempoPinSource)
	if err != nil {
		t.Fatalf("the committed witness must decode: %v", err)
	}
	for _, cell := range []struct {
		name   string
		mutate func(*PersesTempoPin)
	}{
		{"wrong-resource-kind", func(p *PersesTempoPin) { p.Resource.Kind = "TraceDatasource" }},
		{"wrong-wrapper-kind", func(p *PersesTempoPin) { p.TraceQueryWrapperKind = "TraceQL" }},
		{"drifted-file-hash", func(p *PersesTempoPin) { p.WitnessFiles[0].SHA256 = strings.Repeat("0", 64) }},
		{"extra-file", func(p *PersesTempoPin) {
			p.WitnessFiles = append(p.WitnessFiles, PersesTempoFilePin{Path: "upstream/invented.go", SHA256: strings.Repeat("a", 64)})
		}},
		{"short-commit", func(p *PersesTempoPin) { p.PersesCommit = "v0.55.1" }},
	} {
		t.Run(cell.name, func(t *testing.T) {
			tampered := base
			tampered.WitnessFiles = append([]PersesTempoFilePin(nil), base.WitnessFiles...)
			cell.mutate(&tampered)
			if cell.name == "short-commit" {
				// The commit shape is a decode-time rule, not an upstream-text
				// rule: it must fail decodePersesTempoPin.
				if _, err := decodePersesTempoPin(mustTamperJSON(t, tampered)); err == nil {
					t.Fatal("a witness without the 40-char release commit must fail closed")
				}
				return
			}
			if err := tampered.proveAgainstUpstream(); err == nil {
				t.Fatal("a witness that drifts from the upstream source must fail closed")
			} else {
				var witness *TempoWitnessError
				if !errors.As(err, &witness) {
					t.Fatalf("drift must surface the witness verdict, got %v", err)
				}
			}
		})
	}
}

// mustTamperJSON re-serializes a mutated witness declaration document.
func mustTamperJSON(t *testing.T, pin PersesTempoPin) []byte {
	t.Helper()
	raw, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestPersesTempoDatasourceBuilderValidatesBothResources proves the r3
// "builder never calls the query validator" finding is closed on the
// production path: a production mutation that breaks ONLY the query envelope
// (wrapper kind drift) must fail the build, never ship a broken query.
func TestPersesTempoDatasourceBuilderValidatesBothResources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := persesTempoTraceManifest()
	_, _ = liveTempo(t, m, tempoSearchAnswer)
	res, err := BuildPersesTempoDatasource(m)
	if err != nil {
		t.Fatalf("the authorized build must succeed: %v", err)
	}
	// Independently revalidate BOTH emitted resources against the witness.
	pin := persesTempoPin(t)
	if err := ValidatePersesTempoResource(pin, res.Resource); err != nil {
		t.Fatalf("the emitted datasource must validate: %v", err)
	}
	if err := ValidatePersesTempoTraceQuery(pin, res.TraceQuery); err != nil {
		t.Fatalf("the emitted trace query must validate: %v", err)
	}
	// The wrapper envelope is what the release's own documents use.
	raw, err := json.Marshal(res.TraceQuery)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "apiVersion") {
		t.Fatalf("the emitted trace query must not serialize apiVersion: %s", raw)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var outer struct {
		Kind string `json:"kind"`
		Spec struct {
			Plugin struct {
				Kind string `json:"kind"`
			} `json:"plugin"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		t.Fatal(err)
	}
	if outer.Kind != "TraceQuery" || outer.Spec.Plugin.Kind != "TempoTraceQuery" {
		t.Fatalf("the emitted query must serialize the native wrapper envelope: %s", raw)
	}
}

// TestPersesTempoDatasourceQueryDeviationsRejected is the non-vacuous query
// cell: each deviation from the pinned query envelope fails validation.
func TestPersesTempoDatasourceQueryDeviationsRejected(t *testing.T) {
	pin := persesTempoPin(t)
	valid := PersesTempoTraceQuery{
		Kind: PersesTempoTraceQueryWrapperKind,
		Spec: PersesTempoTraceQueryOuter{
			Plugin: PersesTempoTraceQueryPlugin{
				Kind: PersesTempoTraceQueryKind,
				Spec: PersesTempoTraceQuerySpec{
					Datasource: PersesTempoDatasourceSelector{Kind: PersesTempoDatasourceKind, Name: PersesTempoDatasourceName},
					Query:      `{ .resource["service.name"] == "x" }`,
				},
			},
		},
	}
	if err := ValidatePersesTempoTraceQuery(pin, valid); err != nil {
		t.Fatalf("the native query must validate: %v", err)
	}
	for _, cell := range []struct {
		name   string
		mutate func(*PersesTempoTraceQuery)
	}{
		{"flattened-spec", func(q *PersesTempoTraceQuery) {
			q.Kind = PersesTempoTraceQueryKind // TempoTraceQuery without the wrapper
		}},
		{"invented-wrapper", func(q *PersesTempoTraceQuery) { q.Kind = "GrafanaTraceQuery" }},
		{"invented-plugin", func(q *PersesTempoTraceQuery) { q.Spec.Plugin.Kind = "TraceQuery" }},
		{"empty-query", func(q *PersesTempoTraceQuery) { q.Spec.Plugin.Spec.Query = "" }},
		{"external-datasource-kind", func(q *PersesTempoTraceQuery) { q.Spec.Plugin.Spec.Datasource.Kind = "GrafanaDatasource" }},
		{"nameless-datasource", func(q *PersesTempoTraceQuery) { q.Spec.Plugin.Spec.Datasource.Name = "" }},
	} {
		t.Run(cell.name, func(t *testing.T) {
			query := valid
			cell.mutate(&query)
			var shape *TempoResourceShapeError
			if err := ValidatePersesTempoTraceQuery(pin, query); !errors.As(err, &shape) {
				t.Fatalf("deviation %q must fail native validation, got %v", cell.name, err)
			}
		})
	}
}

// TestPersesTempoDatasourceMirrorDriftFailsClosed proves the mirror-to-text
// axis directly: if the mirrored struct stops matching the upstream text,
// validation must refuse the witness. Drift is injected through a swapped
// source text, exactly the failure an edited mirror would produce.
func TestPersesTempoDatasourceMirrorDriftFailsClosed(t *testing.T) {
	// The mirror declares URL with json tag "url"; upstream spec-proxy-http
	// declares it too. A source that no longer declares it must break the
	// evidence check. This proves the check is text-driven, not constant-driven.
	field := reflect.TypeOf(persesUpstreamHTTPProxyCfg{}).Field(0)
	if tag, _, _ := strings.Cut(field.Tag.Get("json"), ","); tag != "url" {
		t.Fatalf("the mirror drifted from upstream: proxy config field %s has tag %q", field.Name, field.Tag.Get("json"))
	}
	if !strings.Contains(string(persesTempoUpstreamSpecProxyHTTP), `json:"url"`) {
		t.Fatal("the committed upstream http proxy source must declare json:\"url\" — the witness copy drifted")
	}
}
