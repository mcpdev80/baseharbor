package podman

import "testing"

func TestQuadletRuntimeResourceMissing(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		message string
		want    bool
	}{
		{"network", "Error: unable to find network with name or ID demo_default: network not found", true},
		{"network", "Error: no such network demo_default", true},
		{"container", "Error: no such container demo-app", true},
		{"volume", "Error: demo-data volume not found", true},
		{"network", "Error: network is being used", false},
		{"network", "", false},
	} {
		if got := quadletRuntimeResourceMissing(tc.kind, tc.message); got != tc.want {
			t.Fatalf("quadletRuntimeResourceMissing(%q, %q) = %t, want %t", tc.kind, tc.message, got, tc.want)
		}
	}
}
