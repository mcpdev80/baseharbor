package main

import (
	"context"
	"testing"
)

func TestMachineLifecycleContextDetachesRequestCancellation(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	lifecycleCtx := machineLifecycleContext(requestCtx)
	select {
	case <-lifecycleCtx.Done():
		t.Fatalf("lifecycle context inherited request cancellation: %v", lifecycleCtx.Err())
	default:
	}
}
