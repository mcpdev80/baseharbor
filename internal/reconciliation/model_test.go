package reconciliation

import "testing"

func TestEvaluateBaseHarborLifecycle(t *testing.T) {
	desired := Desired{Exists: true, Digest: "v2", Owner: OwnershipBaseHarbor}
	cases := []struct {
		name     string
		observed Observed
		state    State
		action   Action
	}{
		{"missing", Observed{Owner: OwnershipBaseHarbor}, StateMissing, ActionCreate},
		{"in-sync", Observed{Exists: true, Digest: "v2", Owner: OwnershipBaseHarbor}, StateInSync, ActionNoop},
		{"drift", Observed{Exists: true, Digest: "v1", Owner: OwnershipBaseHarbor}, StateDrift, ActionRepair},
		{"foreign", Observed{Exists: true, Digest: "v1", Owner: OwnershipForeign}, StateForeignOwnership, ActionBlocked},
		{"conflict", Observed{Exists: true, Digest: "v1", Owner: OwnershipBaseHarbor, Conflict: true}, StateConflict, ActionBlocked},
		{"unsupported", Observed{Owner: OwnershipBaseHarbor, Unsupported: true}, StateUnsupported, ActionBlocked},
		{"degraded", Observed{Exists: true, Digest: "v2", Owner: OwnershipBaseHarbor, Degraded: true}, StateDegraded, ActionBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(desired, tc.observed)
			if got.State != tc.state || got.Action != tc.action {
				t.Fatalf("Evaluate() = %#v, want state=%s action=%s", got, tc.state, tc.action)
			}
		})
	}
}

func TestEvaluateExternalOwnerNeverMutates(t *testing.T) {
	desired := Desired{Exists: true, Digest: "v2", Owner: OwnershipExternal}
	for _, observed := range []Observed{
		{Owner: OwnershipExternal},
		{Exists: true, Digest: "v1", Owner: OwnershipExternal},
		{Exists: true, Digest: "v2", Owner: OwnershipExternal},
	} {
		got := Evaluate(desired, observed)
		if got.Action != ActionObserve {
			t.Fatalf("external reconciliation action = %q, want observe", got.Action)
		}
	}
}

func TestEvaluateDestroyIsOwnershipSafe(t *testing.T) {
	desired := Desired{Exists: false, Owner: OwnershipBaseHarbor}
	owned := Evaluate(desired, Observed{Exists: true, Owner: OwnershipBaseHarbor})
	if owned.Action != ActionDestroy {
		t.Fatalf("owned destroy action = %q", owned.Action)
	}
	foreign := Evaluate(desired, Observed{Exists: true, Owner: OwnershipForeign})
	if foreign.State != StateForeignOwnership || foreign.Action != ActionBlocked {
		t.Fatalf("foreign destroy = %#v", foreign)
	}
}

func TestVerificationTransitionsAreTyped(t *testing.T) {
	base := Result{State: StateInSync, Action: ActionNoop}
	degraded := MarkDegraded(base, "readiness failed")
	if degraded.State != StateDegraded || degraded.Action != ActionBlocked {
		t.Fatalf("degraded = %#v", degraded)
	}
	recovered := MarkVerified(degraded)
	if recovered.State != StateInSync || recovered.Message != "" {
		t.Fatalf("recovered = %#v", recovered)
	}
}
