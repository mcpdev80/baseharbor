package contracts

import (
	"encoding/json"
	"testing"
)

func TestComposePhaseWireRejectsBroadOrAmbiguousActivation(t *testing.T) {
	raw, err := ReadTargetAccessGoldenFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Wire json.RawMessage `json:"wire"`
	}
	if json.Unmarshal(raw, &fixtures) != nil {
		t.Fatal("invalid fixtures")
	}
	var valid json.RawMessage
	for _, fixture := range fixtures {
		var record map[string]any
		if json.Unmarshal(fixture.Wire, &record) == nil && record["request_id"] == "request-compose-phase" {
			valid = fixture.Wire
		}
	}
	if valid == nil || ValidateTargetAccessRecord("request", valid) != nil {
		t.Fatal("missing positive phase fixture")
	}
	for _, scenario := range []string{"empty", "duplicate", "flag", "build", "orphans"} {
		var record map[string]any
		_ = json.Unmarshal(valid, &record)
		payload := record["payload"].(map[string]any)
		switch scenario {
		case "empty":
			payload["services"] = []string{}
		case "duplicate":
			payload["services"] = []string{"postgres", "postgres"}
		case "flag":
			payload["services"] = []string{"--build"}
		case "build":
			payload["build"] = true
		case "orphans":
			payload["remove_orphans"] = true
		}
		encoded, _ := json.Marshal(record)
		if ValidateTargetAccessRecord("request", encoded) == nil {
			t.Fatal("unsafe phase accepted", scenario)
		}
	}
}
