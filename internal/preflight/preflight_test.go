package preflight

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunWithTimeoutRefreshesDeadlineForEveryCheck(t *testing.T) {
	var firstDeadline time.Time
	var secondDeadline time.Time

	checks := []Check{
		{
			Name: "slow first check",
			Run: func(ctx context.Context) error {
				var ok bool
				firstDeadline, ok = ctx.Deadline()
				if !ok {
					t.Fatal("first check has no deadline")
				}
				time.Sleep(20 * time.Millisecond)
				return nil
			},
		},
		{
			Name: "second check",
			Run: func(ctx context.Context) error {
				var ok bool
				secondDeadline, ok = ctx.Deadline()
				if !ok {
					t.Fatal("second check has no deadline")
				}
				return ctx.Err()
			},
		},
	}

	results, ok := RunWithTimeout(context.Background(), checks, 100*time.Millisecond)
	if !ok {
		t.Fatalf("expected all checks to pass: %#v", results)
	}
	if !secondDeadline.After(firstDeadline) {
		t.Fatalf("second check deadline %s must be refreshed after first deadline %s", secondDeadline, firstDeadline)
	}
}

func TestRunPreservesTypedErrorIdentity(t *testing.T) {
	want := errors.New("typed preflight cause")
	results, ok := Run(context.Background(), []Check{{
		Name: "typed",
		Run:  func(context.Context) error { return want },
	}})
	if ok || len(results) != 1 {
		t.Fatalf("unexpected result: ok=%v results=%#v", ok, results)
	}
	if !errors.Is(results[0].Err, want) {
		t.Fatalf("preflight error identity was lost: %#v", results[0])
	}
}
