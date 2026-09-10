package runtime

import "testing"

func TestParseComposeServiceStatesArray(t *testing.T) {
	states, err := parseComposeServiceStates(`[
		{"Service":"api","State":"running","Health":"healthy"},
		{"Service":"worker","State":"running","Health":""},
		{"Service":"edge","State":"running","Health":"unhealthy"}
	]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}
	if !states[0].Ready() || !states[1].Ready() || states[2].Ready() {
		t.Fatalf("unexpected readiness: %#v", states)
	}
}

func TestParseComposeServiceStatesJSONLines(t *testing.T) {
	states, err := parseComposeServiceStates("{\"Service\":\"api\",\"State\":\"running\",\"Health\":\"starting\"}\n{\"Service\":\"worker\",\"State\":\"exited\",\"Health\":\"\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if states[0].Ready() || states[1].Ready() {
		t.Fatalf("starting or exited services must not be ready: %#v", states)
	}
}

func TestParseComposeServiceStatesInvalidJSON(t *testing.T) {
	if _, err := parseComposeServiceStates("not-json"); err == nil {
		t.Fatal("expected invalid compose ps output to fail")
	}
}
