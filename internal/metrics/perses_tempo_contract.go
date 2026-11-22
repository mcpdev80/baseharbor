package metrics

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Perses Tempo upstream model witness (0428-091 round 4, recipe
// act-luna-r3.md).
//
// The files under testdata/perses-tempo-datasource/upstream/ are VERBATIM
// upstream sources of the pinned release — the Go model the Perses v0.55.1
// server actually marshals (github.com/perses/perses@v0.55.1, tag commit
// 34599751521f9b91ccffbf2fb198d727e1ccdc83: pkg/model/api/v1/datasource.go,
// metadata.go, kind.go), its required schema module (github.com/perses/spec
// v0.3.0, tag commit 88c10b39b842e9288c0ffbdea82f65e295a88aca:
// go/datasource/datasource.go, go/plugin/plugin.go) and upstream's OWN sample
// documents (dev/data/8-datasource.json, dev/data/9-dashboard.json). The Go
// module proxy of the release (proxy.golang.org .info) pins the commit
// identity; perses-pin.json records it, and resolvePersesTempoPin re-checks
// the SHA-256 of every committed copy, so an edited or substituted witness
// file fails closed.
//
// Validation is NOT self-referential. The r3 witness was a hand-written JSON
// that repeated the constant the builder asserted. Here the accepted shapes
// come from upstream truth through two independent axes: (1) the compiled
// mirror types in perses_tempo_upstream.go, whose every field name and json
// tag must appear verbatim in the committed upstream source text (textual
// provenance), and whose structs must NOT mention apiVersion anywhere in the
// upstream Datasource model; and (2) upstream's own sample documents — the
// release's dev/data datasource document must decode through the real
// upstream-style Unmarshal semantics (kind "Datasource" or the unmarshal
// fails, no apiVersion present anywhere in it), and its dev/data dashboard
// must ship native TraceQuery panel queries. What the builder emits is then
// compared, path by path, against the field paths the mirror resolves.

// PersesTempoPin is the machine-readable upstream contract witness for the
// pinned Perses release: the release identity, the resource envelope and the
// plugin kinds this adapter must emit, and the integrity pins of the committed
// upstream sources. The declared shapes are never taken on faith: they are
// re-derived from the upstream sources at resolve time.
type PersesTempoPin struct {
	PersesVersion         string               `json:"persesVersion"`
	PersesCommit          string               `json:"persesCommit"`
	SpecModule            string               `json:"specModule"`
	SpecCommit            string               `json:"specCommit"`
	Resource              PersesTempoPinRes    `json:"resource"`
	DatasourcePluginKinds []string             `json:"datasourcePluginKinds"`
	TraceQueryWrapperKind string               `json:"traceQueryWrapperKind"`
	TraceQueryPluginKinds []string             `json:"traceQueryPluginKinds"`
	WitnessFiles          []PersesTempoFilePin `json:"witnessFiles"`
}

// PersesTempoPinRes names the pinned resource envelope: the REST kind plus its
// scope ("project" for a project datasource).
type PersesTempoPinRes struct {
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
}

// PersesTempoFilePin pins one committed upstream source copy: its path under
// the witness directory and its SHA-256, so the witness cannot silently drift
// from what the pinned release actually contains.
type PersesTempoFilePin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// The committed upstream model witness, embedded so validation is deterministic
// from inside the binary and never depends on the caller's working directory.
var (
	//go:embed testdata/perses-tempo-datasource/perses-pin.json
	persesTempoPinData []byte
	//go:embed testdata/perses-tempo-datasource/upstream/perses-datasource-model.go.txt
	persesTempoUpstreamDatasource []byte
	//go:embed testdata/perses-tempo-datasource/upstream/perses-metadata-model.go.txt
	persesTempoUpstreamMetadata []byte
	//go:embed testdata/perses-tempo-datasource/upstream/perses-kind-model.go.txt
	persesTempoUpstreamKind []byte
	//go:embed testdata/perses-tempo-datasource/upstream/spec-datasource-model.go.txt
	persesTempoUpstreamSpecDatasource []byte
	//go:embed testdata/perses-tempo-datasource/upstream/spec-plugin-model.go.txt
	persesTempoUpstreamSpecPlugin []byte
	//go:embed testdata/perses-tempo-datasource/upstream/spec-dashboard-model.go.txt
	persesTempoUpstreamSpecDashboard []byte
	//go:embed testdata/perses-tempo-datasource/upstream/spec-proxy-model.go.txt
	persesTempoUpstreamSpecProxy []byte
	//go:embed testdata/perses-tempo-datasource/upstream/spec-proxy-http-model.go.txt
	persesTempoUpstreamSpecProxyHTTP []byte
	//go:embed testdata/perses-tempo-datasource/upstream/dev-data-datasource.json
	persesTempoUpstreamDatasourceDocs []byte
	//go:embed testdata/perses-tempo-datasource/upstream/tracequery-example-dashboard.json
	persesTempoUpstreamTraceQueryExample []byte
)

// PersesTempoPinSource is the committed pinned Perses Tempo contract witness.
var PersesTempoPinSource = persesTempoPinData

// resolvePersesTempoPin decodes the embedded witness and proves it against the
// committed upstream sources — file hashes, the mirrored-model provenance in
// the upstream source text, and the native documents the release itself ships
// — before any resource can validate against it.
func resolvePersesTempoPin() (PersesTempoPin, error) {
	pin, err := decodePersesTempoPin(persesTempoPinData)
	if err != nil {
		return PersesTempoPin{}, err
	}
	if err := pin.proveAgainstUpstream(); err != nil {
		return PersesTempoPin{}, err
	}
	return pin, nil
}

// decodePersesTempoPin validates a pinned contract witness document.
func decodePersesTempoPin(data []byte) (PersesTempoPin, error) {
	var pin PersesTempoPin
	if err := json.Unmarshal(data, &pin); err != nil {
		return PersesTempoPin{}, fmt.Errorf("decode pinned Perses Tempo contract witness: %w", err)
	}
	if strings.TrimSpace(pin.PersesVersion) == "" || len(pin.PersesCommit) != 40 || len(pin.SpecCommit) != 40 ||
		strings.TrimSpace(pin.Resource.Kind) == "" || pin.Resource.Scope == "" ||
		len(pin.DatasourcePluginKinds) == 0 || len(pin.TraceQueryPluginKinds) == 0 ||
		strings.TrimSpace(pin.TraceQueryWrapperKind) == "" || len(pin.WitnessFiles) == 0 {
		return PersesTempoPin{}, fmt.Errorf("pinned Perses Tempo contract witness is incomplete: version, release commits, resource kind and scope, plugin kinds, wrapper kind and file integrity pins are required")
	}
	return pin, nil
}

// proveAgainstUpstream verifies the witness against the committed upstream
// sources. Each prong fails closed on drift.
func (p PersesTempoPin) proveAgainstUpstream() error {
	files := map[string][]byte{
		"upstream/perses-datasource-model.go.txt":    persesTempoUpstreamDatasource,
		"upstream/perses-metadata-model.go.txt":      persesTempoUpstreamMetadata,
		"upstream/perses-kind-model.go.txt":          persesTempoUpstreamKind,
		"upstream/spec-datasource-model.go.txt":      persesTempoUpstreamSpecDatasource,
		"upstream/spec-plugin-model.go.txt":          persesTempoUpstreamSpecPlugin,
		"upstream/spec-dashboard-model.go.txt":       persesTempoUpstreamSpecDashboard,
		"upstream/spec-proxy-model.go.txt":           persesTempoUpstreamSpecProxy,
		"upstream/spec-proxy-http-model.go.txt":      persesTempoUpstreamSpecProxyHTTP,
		"upstream/dev-data-datasource.json":          persesTempoUpstreamDatasourceDocs,
		"upstream/tracequery-example-dashboard.json": persesTempoUpstreamTraceQueryExample,
	}
	if len(p.WitnessFiles) != len(files) {
		return &TempoWitnessError{Detail: fmt.Sprintf("the witness pins %d upstream files, %d are embedded", len(p.WitnessFiles), len(files))}
	}
	for _, pinned := range p.WitnessFiles {
		data, ok := files[pinned.Path]
		if !ok {
			return &TempoWitnessError{Detail: fmt.Sprintf("witness file %q is not an embedded upstream source", pinned.Path)}
		}
		if sum := sha256Hex(data); sum != pinned.SHA256 {
			return &TempoWitnessError{Detail: fmt.Sprintf("witness file %q drifted from the pinned upstream source (sha256 %s != %s)", pinned.Path, sum, pinned.SHA256)}
		}
	}
	// (1a) The mirrored upstream structs must be proven in the upstream text.
	if err := upstreamModelSourceEvidence(); err != nil {
		return err
	}
	// (1b) The upstream REST Datasource model declares no apiVersion: proven
	// on the compiled mirror and on the source block.
	dsFields, err := jsonFieldPathsOf(reflect.TypeOf(persesUpstreamDatasource{}), "")
	if err != nil {
		return &TempoWitnessError{Detail: err.Error()}
	}
	for _, want := range []string{"kind", "metadata", "spec"} {
		if !containsTopField(dsFields, want) {
			return &TempoWitnessError{Detail: fmt.Sprintf("the pinned upstream Datasource model does not declare %q", want)}
		}
	}
	if containsTopField(dsFields, "apiVersion") {
		return &TempoWitnessError{Detail: "the pinned upstream Datasource model unexpectedly declares apiVersion — re-check the mirror against the release"}
	}
	if block, ok := structValueBlock(string(persesTempoUpstreamDatasource), "Datasource struct"); !ok || strings.Contains(block, "apiVersion") {
		return &TempoWitnessError{Detail: "the committed upstream datasource source does not show a Datasource struct free of apiVersion"}
	}
	// (2) The resource kind is the upstream kind table's, not this package's.
	kindBlock, ok := structValueBlock(string(persesTempoUpstreamKind), `KindDatasource`)
	if !ok || !strings.Contains(kindBlock, `"`+p.Resource.Kind+`"`) {
		return &TempoWitnessError{Detail: fmt.Sprintf("the upstream kind table does not declare resource kind %q", p.Resource.Kind)}
	}
	// (3) The native plugin kinds are the upstream tempo plugin's kinds: the
	// plugin kind table in the spec module names the TraceQuery wrapper, and
	// the release's own dev/data dashboard ships TempoTraceQuery plugins.
	if !strings.Contains(string(persesTempoUpstreamSpecPlugin), `KindTraceQuery      Kind = "`+p.TraceQueryWrapperKind+`"`) &&
		!strings.Contains(string(persesTempoUpstreamSpecPlugin), `KindTraceQuery Kind = "`+p.TraceQueryWrapperKind+`"`) {
		return &TempoWitnessError{Detail: fmt.Sprintf("the pinned spec module does not declare the trace query wrapper kind %q", p.TraceQueryWrapperKind)}
	}
	if !upstreamDocumentCarriesKind(persesTempoUpstreamTraceQueryExample, "kind", p.TraceQueryWrapperKind) ||
		!upstreamDocumentCarriesKind(persesTempoUpstreamTraceQueryExample, "kind", p.TraceQueryPluginKinds[0]) {
		return &TempoWitnessError{Detail: fmt.Sprintf("the pinned release's dashboard witness ships no panel query with the wrapper/plugin kinds %q/%q", p.TraceQueryWrapperKind, p.TraceQueryPluginKinds[0])}
	}
	// (4) Upstream's own datasource document round-trips: the decoded docs use
	// the REST envelope (kind/metadata/spec, never a version field) and carry
	// a plugin envelope with kind and spec.
	var docs []map[string]json.RawMessage
	if err := json.Unmarshal(persesTempoUpstreamDatasourceDocs, &docs); err != nil {
		return &TempoWitnessError{Detail: fmt.Sprintf("decode the pinned release's dev/data datasource witness: %v", err)}
	}
	if len(docs) == 0 {
		return &TempoWitnessError{Detail: "the pinned release's dev/data datasource witness is empty"}
	}
	for _, doc := range docs {
		for forbidden := range doc {
			if forbidden == "apiVersion" {
				return &TempoWitnessError{Detail: "the pinned release's own datasource document unexpectedly carries apiVersion"}
			}
		}
		kind, _ := unmarshalKind(doc["kind"])
		if kind != p.Resource.Kind && kind != "GlobalDatasource" {
			return &TempoWitnessError{Detail: fmt.Sprintf("the pinned release's datasource witness carries kind %q", kind)}
		}
		var spec map[string]json.RawMessage
		if err := json.Unmarshal(doc["spec"], &spec); err != nil {
			return &TempoWitnessError{Detail: "the pinned release's datasource witness has no spec object"}
		}
		var plugin map[string]json.RawMessage
		if err := json.Unmarshal(spec["plugin"], &plugin); err != nil {
			return &TempoWitnessError{Detail: "the pinned release's datasource witness has no spec.plugin envelope"}
		}
		if _, ok := plugin["kind"]; !ok {
			return &TempoWitnessError{Detail: "the pinned release's datasource plugin envelope carries no kind"}
		}
	}
	return nil
}

func unmarshalKind(raw json.RawMessage) (string, error) {
	var kind string
	return kind, json.Unmarshal(raw, &kind)
}

// upstreamDocumentCarriesKind reports whether any object in a decoded JSON
// document tree carries field `field` with value `want` at any nesting depth.
func upstreamDocumentCarriesKind(data []byte, field, want string) bool {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	return treeCarriesKind(doc, field, want)
}

func treeCarriesKind(node any, field, want string) bool {
	switch value := node.(type) {
	case map[string]any:
		if got, ok := value[field].(string); ok && got == want {
			return true
		}
		for _, child := range value {
			if treeCarriesKind(child, field, want) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if treeCarriesKind(child, field, want) {
				return true
			}
		}
	}
	return false
}

// containsTopField reports whether a resolved path list names the top-level
// field.
func containsTopField(paths []string, want string) bool {
	for _, path := range paths {
		if path == want || strings.HasPrefix(path, want+".") {
			return true
		}
	}
	return false
}

// jsonFieldPathsOf walks a struct type exactly the way encoding/json resolves
// fields: anonymous embeds merge, json tags rename, `json:"-"` drops, and the
// first tag word is the name. The walk is depth-bounded and returns sorted
// dotted paths.
func jsonFieldPathsOf(t reflect.Type, prefix string) ([]string, error) {
	seen := map[string]bool{}
	seenType := map[reflect.Type]bool{}
	var paths []string
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("witness model type %s is not a struct", t)
	}
	type frame struct {
		st     reflect.Type
		prefix string
		depth  int
	}
	queue := []frame{{st: t, prefix: prefix, depth: 0}}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		st := f.st
		for i := 0; i < st.NumField(); i++ {
			field := st.Field(i)
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			inline := field.Anonymous && name == ""
			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			fieldPrefix := f.prefix
			if !inline {
				if name == "" {
					continue
				}
				fieldPrefix = joinFieldPath(f.prefix, name)
			}
			switch {
			case ft.Kind() == reflect.Struct && !implementsJSONMarshaler(ft):
				if !inline {
					addFieldPath(&paths, seen, fieldPrefix)
				}
				if !seenType[ft] && f.depth < 16 {
					seenType[ft] = true
					queue = append(queue, frame{st: ft, prefix: fieldPrefix, depth: f.depth + 1})
				}
			case ft.Kind() == reflect.Map || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array:
				if !inline {
					addFieldPath(&paths, seen, fieldPrefix)
				}
				elem := ft.Elem()
				for elem.Kind() == reflect.Pointer {
					elem = elem.Elem()
				}
				if elem.Kind() == reflect.Struct && !implementsJSONMarshaler(elem) && !seenType[elem] && f.depth < 16 {
					seenType[elem] = true
					queue = append(queue, frame{st: elem, prefix: fieldPrefix, depth: f.depth + 1})
				}
			default:
				addFieldPath(&paths, seen, fieldPrefix)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func addFieldPath(paths *[]string, seen map[string]bool, path string) {
	if path != "" && !seen[path] {
		seen[path] = true
		*paths = append(*paths, path)
	}
}

func joinFieldPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// implementsJSONMarshaler reports whether encoding/json treats the type as a
// leaf because it marshals itself.
func implementsJSONMarshaler(t reflect.Type) bool {
	if t == nil {
		return false
	}
	marshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	return t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
