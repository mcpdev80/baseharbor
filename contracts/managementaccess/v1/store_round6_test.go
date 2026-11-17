package managementaccess_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

const canonicalGoldenSHA256 = "255be16cff43839ca1a0b17cc60bc8acee7c518f4616dd4467ea82023bfd9e34"

func TestRecipe6CanonicalGoldenBytesAndFieldOrder(t *testing.T) {
	data, err := os.ReadFile("store.golden.json")
	if err != nil {
		t.Fatalf("read canonical golden: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != canonicalGoldenSHA256 {
		t.Fatalf("canonical golden bytes changed: sha256=%s want=%s", got, canonicalGoldenSHA256)
	}
	container, err := managementaccess.DecodeContainerForTest(data)
	if err != nil {
		t.Fatalf("decode canonical golden: %v", err)
	}
	encoded, err := managementaccess.ExportContainerJSONForTest(container)
	if err != nil {
		t.Fatalf("re-encode canonical golden: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(encoded), bytes.TrimSpace(data)) {
		t.Fatal("canonical golden bytes differ from the codec's canonical encoding")
	}

	assertOrderedFields(t, string(data), "{", "", []string{"version", "dest_path", "records", "timestamp"})
	recordStart := strings.Index(string(data), `"records": [`)
	if recordStart < 0 {
		t.Fatal("canonical golden is missing records")
	}
	records := string(data[recordStart:])
	recordEnd := strings.Index(records, "\n  ],\n  \"timestamp\"")
	if recordEnd < 0 {
		t.Fatal("canonical golden record array boundary not found")
	}
	record := records[:recordEnd]
	assertOrderedFields(t, record, "{\n", "", []string{
		"version", "kind", "installation_id", "trust", "recipient", "scope",
		"not_before", "not_after", "import", "storage", "export",
	})
	assertOrderedObjectFields(t, record, `"trust": {`, []string{"ca_pem", "sha256"})
	assertOrderedObjectFields(t, record, `"import": {`, []string{"rights_granting", "plugin_execution", "credential_redirect"})
	assertOrderedObjectFields(t, record, `"storage": {`, []string{"class", "durable", "encrypted", "disaster_recovery"})
	assertOrderedObjectFields(t, record, `"export": {`, []string{"algorithm", "encrypted", "nonce"})
}

func assertOrderedFields(t *testing.T, data, startMarker, endMarker string, fields []string) {
	t.Helper()
	start := strings.Index(data, startMarker)
	if start < 0 {
		t.Fatalf("canonical JSON object start %q not found", startMarker)
	}
	object := data[start:]
	if endMarker != "" {
		end := strings.Index(object, endMarker)
		if end < 0 {
			t.Fatalf("canonical JSON object end %q not found", endMarker)
		}
		object = object[:end]
	}
	last := -1
	for _, field := range fields {
		position := strings.Index(object, `"`+field+`"`)
		if position <= last {
			t.Fatalf("canonical field %q is missing or out of order in %q", field, object)
		}
		last = position
	}
}

func assertOrderedObjectFields(t *testing.T, data, objectMarker string, fields []string) {
	t.Helper()
	start := strings.Index(data, objectMarker)
	if start < 0 {
		t.Fatalf("canonical nested object %q not found", objectMarker)
	}
	object := data[start:]
	if end := strings.Index(object, "},"); end >= 0 {
		object = object[:end]
	}
	last := -1
	for _, field := range fields {
		position := strings.Index(object, `"`+field+`"`)
		if position <= last {
			t.Fatalf("canonical nested field %q is missing or out of order in %q", field, object)
		}
		last = position
	}
}
