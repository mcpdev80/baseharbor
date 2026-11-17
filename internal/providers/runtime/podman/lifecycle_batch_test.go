package podman

import (
	"strings"
	"testing"
)

func TestParsePodmanContainerUnits(t *testing.T) {
	owned := []RuntimeContainer{{Name: "bh-one"}, {Name: "bh-two"}}
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{name: "all units", output: "/bh-one|bh-one.service\n/bh-two|bh-two.service\n"},
		{name: "direct and Quadlet", output: "/bh-one|<no value>\n/bh-two|bh-two.service\n"},
		{name: "missing container", output: "/bh-one|bh-one.service\n", wantErr: true},
		{name: "duplicate", output: "/bh-one|bh-one.service\n/bh-one|bh-one.service\n/bh-two|bh-two.service\n", wantErr: true},
		{name: "unexpected foreign", output: "/bh-one|bh-one.service\n/foreign|evil.service\n/bh-two|bh-two.service\n", wantErr: true},
		{name: "malformed", output: "/bh-one\n/bh-two|bh-two.service\n", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			units, err := parsePodmanContainerUnits(tc.output, owned)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parsePodmanContainerUnits() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(units) != len(owned) {
				t.Fatalf("got %d units, want %d", len(units), len(owned))
			}
			if units["bh-two"] != "bh-two.service" {
				t.Fatalf("bh-two Quadlet unit = %q", units["bh-two"])
			}
			if strings.Contains(tc.name, "direct") && units["bh-one"] != "" {
				t.Fatalf("direct container incorrectly mapped to unit %q", units["bh-one"])
			}
		})
	}
}
