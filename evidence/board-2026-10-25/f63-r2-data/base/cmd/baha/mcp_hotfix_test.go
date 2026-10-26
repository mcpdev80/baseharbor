package main

import (
	"context"
	"testing"
	"time"
)

func TestMachineLifecycleContextDetachesRequestCancellationAndStaysBounded(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	lifecycleCtx, cancelLifecycle := machineLifecycleContext(requestCtx)
	defer cancelLifecycle()

	select {
	case <-lifecycleCtx.Done():
		t.Fatalf("lifecycle context inherited request cancellation: %v", lifecycleCtx.Err())
	default:
	}

	deadline, ok := lifecycleCtx.Deadline()
	if !ok {
		t.Fatal("detached lifecycle context must have a server-owned deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > machineLifecycleMaxDuration {
		t.Fatalf("lifecycle deadline remaining = %v, max = %v", remaining, machineLifecycleMaxDuration)
	}
}
