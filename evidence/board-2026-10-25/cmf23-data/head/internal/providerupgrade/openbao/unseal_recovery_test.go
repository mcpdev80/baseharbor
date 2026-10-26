package openbao

import "testing"

func TestPostUnsealRecoveryMustBeObserved(t *testing.T) {
	cases := []struct {
		name  string
		state State
		ok    bool
	}{
		{"healthy", State{Initialized: true, Healthy: true, Sealed: false}, true},
		{"still-sealed", State{Initialized: true, Healthy: false, Sealed: true}, false},
		{"uninitialized", State{Initialized: false, Healthy: true, Sealed: false}, false},
		{"unhealthy", State{Initialized: true, Healthy: false, Sealed: false}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyRecoveredState(tc.state)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected recovered OpenBao admission: %v", err)
			}
		})
	}
}
