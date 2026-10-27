package contracts

import (
	"encoding/json"
	"testing"
)

func TestOfflineRegistryContainsEveryPackagedSchema(t *testing.T) {
	registry, err := SchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry) == 0 {
		t.Fatal("empty registry")
	}
	for id, file := range registry {
		data, err := ReadSchema(id)
		if err != nil || !json.Valid(data) {
			t.Fatalf("%s: invalid packaged schema: %v", file, err)
		}
		got, err := SchemaPath(id)
		if err != nil || got != "contracts/"+file {
			t.Fatalf("%s: wrong immutable-source path %s: %v", id, got, err)
		}
	}
	if _, err := ReadSchema("https://schemas.baseharbor.dev/service/v1/sql.schema.json"); err == nil {
		t.Fatal("obsolete domain alias accepted")
	}
}

func TestSchemaReferenceAuditRejectsMissingForeignAndNestedIDs(t *testing.T) {
	base := Namespace + "service/v1/sql.schema.json"
	document := map[string]any{"$id": base, "$defs": map[string]any{"a/b": true}}
	registry := map[string]map[string]any{base: document}
	for _, ref := range []string{"missing.schema.json", "https://foreign.example/schema", "#/$defs/missing", "#unknown-anchor"} {
		if err := validateReferences(base, map[string]any{"$ref": ref}, registry); err == nil {
			t.Fatalf("unresolved reference %q accepted", ref)
		}
	}
	if err := validateReferences(base, map[string]any{"$ref": "#/$defs/a~1b"}, registry); err != nil {
		t.Fatal(err)
	}
	if err := validateReferences(base, map[string]any{"$defs": map[string]any{"nested": map[string]any{"$id": "other"}}}, registry); err == nil {
		t.Fatal("unregistered nested ID accepted")
	}
}
