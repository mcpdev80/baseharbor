package contracts_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

func TestMachineReadModelArtifactsDeriveFromActualCoreTypes(t *testing.T) {
	schema, err := machinereadmodels.Schema()
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := contracts.ReadSchema(machinereadmodels.SchemaID)
	if err != nil || !bytes.Equal(append(schema, '\n'), shipped) {
		t.Fatal("regenerate schema from actual Core types", err)
	}
	golden, err := machinereadmodels.Golden()
	if err != nil {
		t.Fatal(err)
	}
	shipped, err = os.ReadFile("machine/v1/read-models.golden.json")
	if err != nil || !bytes.Equal(append(golden, '\n'), shipped) {
		t.Fatal("regenerate synthetic Core examples", err)
	}
	for _, example := range machinereadmodels.Examples() {
		validator := readModelSchema(t, schema, example.Operation)
		raw, err := json.Marshal(example.Value)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(value); err != nil {
			t.Fatalf("%s: %v", example.Operation, err)
		}
	}
}

func TestMachineReadModelSchemaRejectsInventedUIFields(t *testing.T) {
	raw, err := machinereadmodels.Schema()
	if err != nil {
		t.Fatal(err)
	}
	validator := readModelSchema(t, raw, "target.list")
	for _, wire := range []string{
		`{"contract_version":"v1","targets":[{"contract_version":"v1","name":"remote","runtime_provider":"podman","access_reference":"node","health":"healthy"}]}`,
		`{"contract_version":"v1","targets":[{"contract_version":"v1","name":"remote","runtime_provider":"podman","access_reference":"node","effective":"yes"}]}`,
		`{"contract_version":"v1","targets":[{"name":"remote"}]}`,
		`{"contract_version":"v1","targets":{},"defaults":{"role":"admin"}}`,
	} {
		var value any
		if err := json.Unmarshal([]byte(wire), &value); err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(value); err == nil {
			t.Fatalf("accepted invented UI model: %s", wire)
		}
	}
}

func readModelSchema(t *testing.T, raw []byte, operation string) *jsonschema.Resolved {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["$ref"] = "#/$defs/" + operation
	selected, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(selected, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
