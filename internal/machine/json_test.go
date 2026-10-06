package machine

import (
	"errors"
	"strings"
	"testing"
)

func TestMachineJSONObjectRejectsAmbiguityAndBounds(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`{"target":"one","target":"two"}`),
		[]byte(`{"input":{"target":"one","target":"two"}}`),
		[]byte(`{"items":[{"id":"one","id":"two"}]}`),
		[]byte(`{"target":"one","\u0074arget":"two"}`),
		[]byte(`{"credential-secret":"sensitive-value","credential-secret":"other"}`),
		[]byte(`{} {}`), []byte(`null`), []byte(`[]`), []byte(`true`),
		[]byte(`{"broken":}`), []byte("{\"raw\":\""+string([]byte{255})+"\"}"),
		[]byte(`{}`+strings.Repeat(" ", 4096)),
		[]byte(`{"nested":`+strings.Repeat("[", 65)+"0"+strings.Repeat("]", 65)+"}"),
	} {
		if err := ValidateJSONObject(data, 4096); !errors.Is(err, ErrJSONObject) ||
			strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "credential") {
			t.Fatal("invalid object accepted or untrusted parser content exposed", err)
		}
	}
	for _, data := range []string{`{}`, `{"items":[{"id":"one"},{"id":"two"}],"nested":{"value":null}}`, `{"large_integer":9007199254740993}`} {
		if err := ValidateJSONObject([]byte(data), 4096); err != nil {
			t.Fatal("valid object rejected", err)
		}
	}
	if !errors.Is(ValidateJSONObject([]byte("{}"), 0), ErrJSONObject) {
		t.Fatal("invalid caller bound accepted")
	}
}
