package policy

import "testing"

func TestAggregate(t *testing.T) {
	if got := Aggregate(nil); got != Allow {
		t.Fatalf("empty=%s", got)
	}
	if got := Aggregate([]Finding{{Decision: Warn}, {Decision: Allow}}); got != Warn {
		t.Fatalf("warn=%s", got)
	}
	if got := Aggregate([]Finding{{Decision: Warn}, {Decision: Deny}}); got != Deny {
		t.Fatalf("deny=%s", got)
	}
}
