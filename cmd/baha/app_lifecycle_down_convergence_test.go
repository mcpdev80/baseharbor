package main

import (
	"context"
	"io"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestDownRequiresConfirmedConsent (0428-060) pins the seam's confirmation
// check itself: only the explicit --yes argument confirms. The production
// behavior of the seam on the real command route is proven by
// TestDownRouteWithoutConsentMakesNoChange and
// TestDownRouteWithConsentRunsRetirement (0428-060 r2, in
// app_lifecycle_down_route_0428_060_test.go), not by this helper check.
func TestDownRequiresConfirmedConsent(t *testing.T) {
	if !downRequiresConfirmedConsent(context.Background(), []string{"--yes"}) {
		t.Fatal("down execution did not honour the explicit --yes confirmation the mission requires")
	}
	if downRequiresConfirmedConsent(context.Background(), nil) {
		t.Fatal("down execution reported consent that was never given")
	}
	if downRequiresConfirmedConsent(context.Background(), []string{"--yessure"}) {
		t.Fatal("a lookalike option must not confirm the down mutation")
	}
}

// TestDownConsentGoesBothWays (0428-060) proves the REAL reachable down path
// ('app down'/'stop' -> newApplicationDestroyExecution -> retireRuntimePreservingData)
// gates on --yes in both directions:
//   - gate active   (--yes absent) -> execution.confirmed == false (refuses)
//   - gate open     (--yes present) -> execution.confirmed == true (allowed)
//
// The r2 review found this helper-level construction is NOT sufficient on its
// own: it never runs the registered command. It is retained as the
// execution-level complement; the command-level proof lives in the r2 file.
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
