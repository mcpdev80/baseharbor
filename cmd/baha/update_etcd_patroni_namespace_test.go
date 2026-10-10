package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestRestoredPatroniNamespaceDoesNotRequireLeasedKeys(t *testing.T) {
	const prefix = "/service/baseharbor-control-postgres/"
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	entry := func(key, value string, lease int64) map[string]any {
		return map[string]any{"key": encode(prefix + key), "value": encode(value), "lease": lease}
	}
	for _, tc := range []struct {
		name    string
		entries []map[string]any
		valid   bool
	}{
		{"durable-only", []map[string]any{entry("initialize", "7542627032619304561", 0), entry("config", `{"ttl":30,"loop_wait":10}`, 0)}, true},
		{"leases-only", []map[string]any{entry("leader", "pg1", 12), entry("members/pg1", `{}`, 12)}, false},
		{"leased-initialize", []map[string]any{entry("initialize", "7542627032619304561", 12), entry("config", `{"ttl":30}`, 0)}, false},
		{"empty-system-id", []map[string]any{entry("initialize", "", 0), entry("config", `{"ttl":30}`, 0)}, false},
		{"invalid-config", []map[string]any{entry("initialize", "7542627032619304561", 0), entry("config", `null`, 0)}, false},
		{"duplicate-key", []map[string]any{entry("initialize", "7542627032619304561", 0), entry("initialize", "1", 0), entry("config", `{"ttl":30}`, 0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"kvs": tc.entries})
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyRestoredPatroniNamespace(data, prefix); (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}
