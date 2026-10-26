package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type recoveringOpenBaoStateRuntime struct {
	bhruntime.RuntimeProvider
	calls int
	ready bool
}

func (r *recoveringOpenBaoStateRuntime) ExecProject(context.Context, string, string, string, string, ...string) (string, error) {
	r.calls++
	if r.ready && r.calls > 1 {
		return `{"initialized":false,"sealed":true,"version":"2.7.0"}`, nil
	}
	return "", errors.New("SQL connection is restarting")
}

func TestOpenBaoReadinessReturnsObservedStateAfterTransientSQLStartup(t *testing.T) {
	runtime := &recoveringOpenBaoStateRuntime{ready: true}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	state, err := waitForOpenBaoStateReady(ctx, runtime, bhruntime.Files{})
	if err != nil || runtime.calls != 2 || state.Initialized || !state.Sealed || state.Version != "2.7.0" {
		t.Fatalf("readiness lost actual seal/bootstrap state: %+v calls=%d err=%v", state, runtime.calls, err)
	}
}

func TestOpenBaoReadinessCannotOutliveCallerDeadline(t *testing.T) {
	runtime := &recoveringOpenBaoStateRuntime{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := waitForOpenBaoStateReady(ctx, runtime, bhruntime.Files{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unready SQL/OpenBao was accepted or deadline lost: %v", err)
	}
}
