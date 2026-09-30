package podman

import (
	"errors"
	"testing"
)

func TestPodmanNetworkHasActiveConsumers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "network in use", err: errors.New("network is being used"), want: true},
		{name: "associated containers", err: errors.New("has associated containers with it"), want: true},
		{name: "active endpoints", err: errors.New("network has active endpoints"), want: true},
		{name: "permission denied", err: errors.New("permission denied"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := podmanNetworkHasActiveConsumers(tc.err); got != tc.want {
				t.Fatalf("podmanNetworkHasActiveConsumers(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}
