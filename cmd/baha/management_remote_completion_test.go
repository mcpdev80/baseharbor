package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRemoteApplyCompletionIsReadableAndNeverDumpsReceipt(t *testing.T) {
	raw := json.RawMessage(`{"contract_version":"v1","operation":"apply","verified":true,"target":"dev-docker-1","application":"demo","environment":"dev","internal":"not-for-human-output","status":{"target":"dev-docker-1","application":"demo","environment":"dev","ready":true,"application_urls":["http://192.168.50.5:43373"],"checks":[{"name":"managed-exposure/web","ok":true,"detail":"https://example.test:28443"},{"name":"managed-exposure/unsafe","ok":true,"detail":"https://user:password@example.test"},{"name":"managed-exposure/duplicate","ok":true,"detail":"https://example.test:28443"},{"name":"management-ui/sql","ok":true,"detail":"https://admin.example.test"}]}}`)
	var output bytes.Buffer
	if err := renderRemoteApplyCompletion(context.Background(), &output, raw); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"READY · demo", "dev-docker-1", "Environment: dev", "Open (IP): http://192.168.50.5:43373", "Open (URL): https://example.test:28443", "baha status", "baha doctor"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing useful completion %q: %s", want, text)
		}
	}
	for _, bad := range []string{"contract_version", "not-for-human-output", "password", "admin.example.test", "{\""} {
		if strings.Contains(text, bad) {
			t.Fatalf("receipt or credential data leaked: %s", text)
		}
	}
	if strings.Count(text, "https://example.test:28443") != 1 {
		t.Fatal("duplicate application link")
	}
	for _, replacement := range []string{`"verified":false`, `"ready":false`} {
		invalid := strings.Replace(string(raw), strings.Split(replacement, ":")[0]+":true", replacement, 1)
		if err := renderRemoteApplyCompletion(context.Background(), new(bytes.Buffer), []byte(invalid)); err == nil {
			t.Fatal("unverified receipt reported READY")
		}
	}
}
