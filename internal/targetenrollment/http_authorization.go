package targetenrollment

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// Grant input is a flat operator DTO. Reject ambiguous keys before normal
// typed decoding; last-key-wins parsing must not choose an authorized scope.
func decodeAuthorizationInput(data []byte) (authorizationInput, error) {
	var input authorizationInput
	if !utf8.Valid(data) {
		return input, ErrDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return input, ErrDenied
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil || len(fields) >= 5 {
			return input, ErrDenied
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return input, ErrDenied
		}
		fields[key] = value
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') {
		return input, ErrDenied
	}
	if _, err := decoder.Token(); err != io.EOF {
		return input, ErrDenied
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return input, ErrDenied
	}
	typed := json.NewDecoder(bytes.NewReader(canonical))
	typed.DisallowUnknownFields()
	if err := typed.Decode(&input); err != nil {
		return authorizationInput{}, ErrDenied
	}
	return input, nil
}
