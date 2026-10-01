package podman

import "testing"

func TestComposeContainerServiceFromExpectedName(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		resource  string
		want      string
		wantMatch bool
	}{
		{name: "default service", project: "bh-demo-dev", resource: "bh-demo-dev-rabbitmq-1", want: "rabbitmq", wantMatch: true},
		{name: "named service", project: "bh-demo-dev", resource: "bh-demo-dev-rabbitmq-access-1", want: "rabbitmq-access", wantMatch: true},
		{name: "service contains dash and number", project: "bh-demo-dev", resource: "bh-demo-dev-worker-1-blue-1", want: "worker-1-blue", wantMatch: true},
		{name: "wrong project", project: "bh-demo-dev", resource: "other-rabbitmq-1", wantMatch: false},
		{name: "not compose identity", project: "bh-demo-dev", resource: "bh-demo-dev-rabbitmq", wantMatch: false},
		{name: "empty service", project: "bh-demo-dev", resource: "bh-demo-dev--1", wantMatch: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := composeContainerServiceFromExpectedName(tc.project, tc.resource)
			if ok != tc.wantMatch || got != tc.want {
				t.Fatalf("composeContainerServiceFromExpectedName(%q, %q) = %q, %v; want %q, %v", tc.project, tc.resource, got, ok, tc.want, tc.wantMatch)
			}
		})
	}
}
