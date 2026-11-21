package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestDownRequiresConfirmedConsent (0428-060) is the down-convergence seam check:
// 'down' must honour the same assume-yes consent the up/apply lifecycle uses so
// an unconfirmed down makes no runtime change. Kept as a named helper so the
// acceptance test asserts the seam directly, both directions.
func TestDownRequiresConfirmedConsent(t *testing.T) {
	if !downRequiresConfirmedConsent(withAssumeYes(context.Background(), true)) {
		t.Fatal("down execution did not honour the assume-yes consent the mission requires")
	}
	if downRequiresConfirmedConsent(context.Background()) {
		t.Fatal("down execution reported consent that was never given")
	}
}

// TestDownConsentGoesBothWays (0428-060) proves the REAL reachable down path
// ('app down'/'stop' -> newApplicationDestroyExecution -> retireRuntimePreservingData)
// gates on --yes in both directions:
//   - gate active   (--yes absent) -> execution.confirmed == false (refuses)
//   - gate open     (--yes present) -> execution.confirmed == true (allowed)
func TestDownConsentGoesBothWays(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	r.Manifest.Services = application.Services{}

	// Direction 1: no --yes -> gate active -> refuses the mutation.
	without, err := newApplicationDestroyExecution(context.Background(), r.Store, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if without.confirmed {
		t.Fatal("down without --yes must be unconfirmed (gate active refuses)")
	}

	// Direction 2: --yes -> gate open -> confirmed.
	with, err := newApplicationDestroyExecution(context.Background(), r.Store, []string{"--yes"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !with.confirmed {
		t.Fatal("down with --yes must be confirmed (gate open allows)")
	}
}
