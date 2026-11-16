package machinehttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// decodeRequest enforces the bound on the entire body, not just the first JSON
// value. Parser diagnostics do not echo untrusted field names or credentials.
func decodeRequest(body io.Reader, destination any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxRequestBytes+1))
	if err != nil || len(data) > maxRequestBytes {
		return errors.New("request body exceeds the limit or cannot be read")
	}
	if err := machine.ValidateJSONObject(data, maxRequestBytes); err != nil {
		return errors.New("request JSON is invalid or ambiguous")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("request JSON is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request requires exactly one JSON object")
	}
	return nil
}
