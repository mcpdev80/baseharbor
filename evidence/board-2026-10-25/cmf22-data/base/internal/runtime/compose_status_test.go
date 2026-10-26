package runtime

import "testing"

func TestParseComposeServiceStatesArray(t *testing.T) {
	states, err := parseComposeServiceStates(`[
		{"Service":"api","State":"running","Health":"healthy"},
		{"Service":"worker","State":"running","Health":""},
		{"Service":"edge","State":"running","Health":"unhealthy","Publishers":[{"URL":"0.0.0.0","TargetPort":443,"PublishedPort":8443,"Protocol":"tcp"}]}
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
	if len(states[2].Publishers) != 1 {
		t.Fatalf("expected one publisher, got %#v", states[2].Publishers)
	}
	publisher := states[2].Publishers[0]
	if publisher.URL != "0.0.0.0" || publisher.TargetPort != 443 || publisher.PublishedPort != 8443 || publisher.Protocol != "tcp" {
		t.Fatalf("unexpected publisher: %#v", publisher)
	}
}

func TestParseComposeServiceStatesJSONLines(t *testing.T) {
	states, err := parseComposeServiceStates("{\"Service\":\"api\",\"State\":\"running\",\"Health\":\"starting\",\"Publishers\":[{\"URL\":\"127.0.0.1\",\"TargetPort\":8080,\"PublishedPort\":18080,\"Protocol\":\"TCP\"}]}\n{\"Service\":\"worker\",\"State\":\"exited\",\"Health\":\"\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if states[0].Ready() || states[1].Ready() {
		t.Fatalf("starting or exited services must not be ready: %#v", states)
	}
	if len(states[0].Publishers) != 1 || states[0].Publishers[0].Protocol != "tcp" {
		t.Fatalf("expected normalized publisher protocol: %#v", states[0].Publishers)
	}
}

func TestParseComposeServiceStatesInvalidJSON(t *testing.T) {
	if _, err := parseComposeServiceStates("not-json"); err == nil {
		t.Fatal("expected invalid compose ps output to fail")
	}
}
