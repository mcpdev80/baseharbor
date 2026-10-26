package runtime

import (
	"errors"
	"testing"
)

func TestNetworkHasActiveConsumers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "docker active endpoints", err: errors.New("error while removing network: network demo has active endpoints"), want: true},
		{name: "podman in use", err: errors.New("network is being used by container 123"), want: true},
		{name: "connected containers", err: errors.New("network has connected containers"), want: true},
		{name: "unrelated runtime error", err: errors.New("permission denied"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := networkHasActiveConsumers(tc.err); got != tc.want {
				t.Fatalf("networkHasActiveConsumers(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
