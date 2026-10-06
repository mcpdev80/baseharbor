package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

type wireFixture struct {
	Record string          `json:"record"`
	Wire   json.RawMessage `json:"wire"`
}

func TestCanonicalTargetAccessGoldenRecordsValidateOffline(t *testing.T) {
	raw, err := ReadTargetAccessGoldenFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []wireFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty canonical fixtures")
	}
	for _, fixture := range fixtures {
		if err := ValidateTargetAccessRecord(fixture.Record, fixture.Wire); err != nil {
			t.Fatalf("%s fixture: %v; schema error: %v", fixture.Record, err, wireCache.err)
		}
	}
}

func TestTargetAccessWireRejectsAmbiguousAndUnsafeRequests(t *testing.T) {
	valid := `{"contract_version":"baseharbor.target-access/v1","protocol_version":"1","request_id":"request-a","correlation_id":"execution-a","target_id":"lab","operation":"runtime.container.stop","issued_at":"2026-10-06T02:00:00Z","deadline_at":"2026-10-06T02:05:00Z","payload":{"resource_id":"owned-a"}}`
	if err := ValidateTargetAccessRecord("request", []byte(valid)); err != nil {
		t.Fatalf("positive control: %v; %v", err, wireCache.err)
	}
	for name, raw := range map[string]string{
		"target-duplicate":     strings.Replace(valid, `"target_id":"lab"`, `"target_id":"foreign","target_id":"lab"`, 1),
		"nested-duplicate":     strings.Replace(valid, `"resource_id":"owned-a"`, `"resource_id":"foreign","resource_id":"owned-a"`, 1),
		"generic-host-command": strings.Replace(valid, "runtime.container.stop", "runtime_command", 1),
		"unknown-secret-field": strings.Replace(valid, `"resource_id":"owned-a"`, `"resource_id":"owned-a","SECRET-CREDENTIAL":"secret"`, 1),
		"missing-correlation":  strings.Replace(valid, `"correlation_id":"execution-a",`, "", 1),
		"missing-deadline":     strings.Replace(valid, `"deadline_at":"2026-10-06T02:05:00Z",`, "", 1),
		"invalid-date":         strings.Replace(valid, "2026-10-06T02:00:00Z", "2026-99-99T02:00:00Z", 1),
		"trailing-record":      valid + valid,
		"null-payload":         strings.Replace(valid, `{"resource_id":"owned-a"}`, "null", 1),
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateTargetAccessRecord("request", []byte(raw))
			if err != ErrTargetAccessWire || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unsafe request accepted or echoed: %v", err)
			}
		})
	}
}

func TestTargetAccessWireRejectsTerminalReplayAndMalformedEvents(t *testing.T) {
	open := `{"contract_version":"baseharbor.target-access/v1","protocol_version":"1","stream_id":"stream-a","correlation_id":"execution-a","target_id":"lab","resource_id":"owned-a","kind":"terminal","deadline_at":"2026-10-06T02:05:00Z","terminal":{"rows":24,"cols":80,"argv":["/bin/sh"]}}`
	if err := ValidateTargetAccessRecord("stream_open", []byte(open)); err != nil {
		t.Fatal(err, wireCache.err)
	}
	for _, bad := range []string{
		strings.Replace(open, `"terminal":`, `"resume_after":0,"terminal":`, 1),
		strings.Replace(open, `"rows":24`, `"rows":513`, 1),
		strings.Replace(open, `"terminal":`, `"logs":{},"terminal":`, 1),
	} {
		if err := ValidateTargetAccessRecord("stream_open", []byte(bad)); err == nil {
			t.Fatal("unsafe terminal open accepted")
		}
	}
	event := `{"contract_version":"baseharbor.target-access/v1","protocol_version":"1","stream_id":"stream-a","correlation_id":"execution-a","sequence":1,"observed_at":"2026-10-06T02:00:00Z","type":"data","data":"aGVsbG8K"}`
	for _, bad := range []string{
		strings.Replace(event, "aGVsbG8K", "%%%%", 1),
		strings.Replace(event, `"sequence":1`, `"sequence":0`, 1),
		strings.Replace(event, `"data":"aGVsbG8K"`, `"data":"aGVsbG8K","exit_code":0`, 1),
		strings.Replace(event, `"type":"data"`, `"type":"exit"`, 1),
	} {
		if err := ValidateTargetAccessRecord("stream_event", []byte(bad)); err == nil {
			t.Fatal("malformed terminal event accepted")
		}
	}
}

func TestTargetAccessWireRejectsUnboundedOrForeignPayloads(t *testing.T) {
	for _, test := range []struct {
		operation string
		payload   string
	}{
		{"runtime.compose.apply", `{"project_directory":"../foreign","files":["compose.yaml"]}`},
		{"artifact.bundle.stage", `{"bundle_id":"safe","files":[{"path":"a/../foreign","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","data":"YQ=="}]}`},
		{"runtime.exec", `{"resource_id":"owned","argv":[]}`},
		{"runtime.volume.remove", `{"name":"--all"}`},
		{"runtime.quadlet.apply", `{"name":"workload","content":"[Container]\\nImage=example/image:fixture"}`},
		{"runtime.quadlet.remove", `{"name":"../foreign.container"}`},
		{"runtime.quadlet.enable", `{"name":"foreign.service"}`},
	} {
		raw := `{"contract_version":"baseharbor.target-access/v1","protocol_version":"1","request_id":"request-a","correlation_id":"execution-a","target_id":"lab","operation":"` + test.operation + `","issued_at":"2026-10-06T02:00:00Z","deadline_at":"2026-10-06T02:05:00Z","payload":` + test.payload + `}`
		if err := ValidateTargetAccessRecord("request", []byte(raw)); err == nil {
			t.Fatalf("unsafe %s payload accepted", test.operation)
		}
	}
	if err := ValidateTargetAccessRecord("request", []byte(strings.Repeat(" ", TargetAccessMaxFrameBytes+1))); err == nil {
		t.Fatal("oversized frame accepted")
	}
	if err := ValidateTargetAccessRecord("request", []byte(strings.Repeat("[", 65)+strings.Repeat("]", 65))); err == nil {
		t.Fatal("unbounded JSON nesting accepted")
	}
}
