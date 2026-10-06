package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed machine/v1/control.golden.json
var machineControlFixtures embed.FS

func ReadMachineControlGoldenFixtures() ([]byte, error) {
	return machineControlFixtures.ReadFile("machine/v1/control.golden.json")
}

var machineRecords = map[string]bool{
	"execution": true, "event": true, "execute_request": true,
	"discovery": true, "error_result": true,
}

var machineControlCache struct {
	sync.Once
	schemas map[string]*jsonschema.Resolved
	err     error
}

// ValidateMachineControlRecord validates the packaged public semantic envelope.
// Operation-specific input and result schemas, operator authorization and policy
// remain the semantic executor's responsibility.
func ValidateMachineControlRecord(record string, data []byte) error {
	invalid := errors.New("invalid machine control record")
	if !machineRecords[record] || len(data) == 0 || len(data) > 4<<20 || !utf8.Valid(data) {
		return invalid
	}
	machineControlCache.Do(resolveMachineControlSchemas)
	if machineControlCache.err != nil {
		return invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	value, err := strictWireValue(decoder, 0)
	if err != nil {
		return invalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid
	}
	if err := machineControlCache.schemas[record].Validate(value); err != nil {
		return invalid
	}
	object, ok := value.(map[string]any)
	if !ok {
		return invalid
	}
	for _, field := range []string{"started_at", "finished_at", "occurred_at"} {
		if raw, present := object[field]; present {
			text, ok := raw.(string)
			if !ok {
				return invalid
			}
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				return invalid
			}
		}
	}
	return nil
}

func resolveMachineControlSchemas() {
	machineControlCache.schemas = map[string]*jsonschema.Resolved{}
	raw, err := ReadSchema(Namespace + "machine/v1/control.schema.json")
	if err != nil {
		machineControlCache.err = err
		return
	}
	for record := range machineRecords {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			machineControlCache.err = err
			return
		}
		document["$ref"] = "#/$defs/" + record
		selected, err := json.Marshal(document)
		if err != nil {
			machineControlCache.err = err
			return
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(selected, &schema); err != nil {
			machineControlCache.err = err
			return
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			machineControlCache.err = err
			return
		}
		machineControlCache.schemas[record] = resolved
	}
}
