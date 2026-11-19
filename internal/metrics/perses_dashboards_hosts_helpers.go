package metrics

import (
	"bytes"
	"encoding/json"
)

// mustJSONHosts marshals v to compact JSON without HTML escaping, and panics
// on error. It is a build helper for building Perses plugin specs from raw
// literals: the inputs are compile-time constants, so marshalling cannot fail
// at runtime. The & character is emitted literally (not \u0026) so the golden
// witnesses are human-readable Perses-native JSON.
//
// This belongs to the host/container dashboard set (0428-077) and is disjoint
// from #1597's perses_dashboards_helpers.go.
func mustJSONHosts(v any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic("perses host panel spec marshal failed: " + err.Error())
	}
	// The encoder appends a trailing newline; trim it for a compact spec.
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}
