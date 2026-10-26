package machine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var ErrJSONObject = errors.New("invalid or ambiguous machine JSON object")

// ValidateJSONObject rejects duplicate keys at every depth before typed decoding.
// The entire object, including trailing whitespace, must fit the caller's bound.
// Error text never contains untrusted keys, values or parser diagnostics.
func ValidateJSONObject(data []byte, maxBytes int) error {
	if maxBytes <= 0 || len(data) == 0 || len(data) > maxBytes || !utf8.Valid(data) {
		return ErrJSONObject
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrJSONObject
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder, 0); err != nil {
		return ErrJSONObject
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrJSONObject
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return ErrJSONObject
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrJSONObject
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok {
				return ErrJSONObject
			}
			if _, exists := keys[name]; exists {
				return ErrJSONObject
			}
			keys[name] = struct{}{}
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return ErrJSONObject
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return ErrJSONObject
		}
	default:
		return ErrJSONObject
	}
	return nil
}
