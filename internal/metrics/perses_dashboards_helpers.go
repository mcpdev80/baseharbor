package metrics

import (
	"encoding/json"
)

// mustJSON marshals v to compact JSON and panics on error. It is a test/build
// helper for building Perses plugin specs from raw literals: the inputs are
// compile-time constants, so marshalling cannot fail at runtime.
func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic("perses panel spec marshal failed: " + err.Error())
	}
	return json.RawMessage(data)
}

// rawJSON wraps a pre-serialized JSON literal as raw JSON. It is used for query
// specs whose Go struct representation would require escaping the PromQL/LogQL/
// TraceQL literals; the literal is a compile-time constant and is validated by
// the golden-file test.
func rawJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}
