package openbao

import "testing"

func TestVerifyHAMembersFailClosed(t *testing.T) {
	good := []State{
		{Version: "2.7.0", Initialized: true, Healthy: true},
		{Version: "2.7.0", Initialized: true, Healthy: true},
		{Version: "2.7.0", Initialized: true, Healthy: true},
	}
	if err := verifyHAMembers(good, 3, "2.7.0"); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func([]State) []State{
		"missing-member":       func(s []State) []State { return s[:2] },
		"mixed-version":        func(s []State) []State { s[1].Version = "2.6.0"; return s },
		"sealed-member":        func(s []State) []State { s[1].Sealed = true; return s },
		"uninitialized-member": func(s []State) []State { s[1].Initialized = false; return s },
		"unhealthy-member":     func(s []State) []State { s[1].Healthy = false; return s },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			states := append([]State(nil), good...)
			if err := verifyHAMembers(mutate(states), 3, "2.7.0"); err == nil {
				t.Fatal("invalid OpenBao HA topology was accepted")
			}
		})
	}
}
