package runtimeoperation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerExecutesAndPersistsAsyncOperation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "operations")
	release := make(chan struct{})
	manager, err := New(dir, map[string]Executor{
		executorKey("object-storage.s3", "runtime.create"): ExecutorFunc(func(ctx context.Context, request Request) (Result, error) {
			select {
			case <-release:
				return Result{ResourceID: "object-storage.s3/" + request.ResourceName, Binding: map[string]any{"bucket": "physical-" + request.ResourceName}}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	request := Request{
		Application: "demo", Capability: "object-storage.s3", Operation: "runtime.create",
		ResourceName: "user-4711", IdempotencyKey: "tenant-4711-storage",
	}
	op, replay, err := manager.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replay || op.State != StatePending {
		t.Fatalf("initial operation = %#v replay=%v", op, replay)
	}

	waitForState(t, manager, op.ID, StateRunning)
	close(release)
	done := waitForState(t, manager, op.ID, StateSucceeded)
	if done.Result.ResourceID != "object-storage.s3/user-4711" {
		t.Fatalf("resource id = %q", done.Result.ResourceID)
	}

	reloaded, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reloaded.Get(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.State != StateSucceeded || persisted.Result.ResourceID != done.Result.ResourceID {
		t.Fatalf("persisted operation = %#v", persisted)
	}
}

func TestManagerIdempotencyReturnsSameOperation(t *testing.T) {
	manager, err := New(t.TempDir(), map[string]Executor{
		executorKey("object-storage.s3", "runtime.create"): ExecutorFunc(func(context.Context, Request) (Result, error) {
			return Result{ResourceID: "resource"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		Application: "demo", Capability: "object-storage.s3", Operation: "runtime.create",
		ResourceName: "tenant", IdempotencyKey: "same-request",
	}
	first, _, err := manager.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, replay, err := manager.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay || first.ID != second.ID {
		t.Fatalf("idempotency replay = %v first=%s second=%s", replay, first.ID, second.ID)
	}
}

func TestManagerFailureIsObservable(t *testing.T) {
	manager, err := New(t.TempDir(), map[string]Executor{
		executorKey("object-storage.s3", "runtime.create"): ExecutorFunc(func(context.Context, Request) (Result, error) {
			return Result{}, errors.New("provider unavailable")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	op, _, err := manager.Submit(context.Background(), Request{
		Application: "demo", Capability: "object-storage.s3", Operation: "runtime.create",
		ResourceName: "tenant", IdempotencyKey: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForState(t, manager, op.ID, StateFailed)
	if failed.Error != "provider unavailable" {
		t.Fatalf("failure = %q", failed.Error)
	}
}

func TestManagerRejectsUnsupportedOperationBeforeQueueing(t *testing.T) {
	manager, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = manager.Submit(context.Background(), Request{
		Application: "demo", Capability: "object-storage.s3", Operation: "runtime.create",
		ResourceName: "tenant", IdempotencyKey: "unsupported",
	})
	if err == nil {
		t.Fatal("expected unsupported capability operation")
	}
}

func TestManagerResumeRequeuesInterruptedOperation(t *testing.T) {
	dir := t.TempDir()
	manager, err := New(dir, map[string]Executor{
		executorKey("object-storage.s3", "runtime.create"): ExecutorFunc(func(context.Context, Request) (Result, error) {
			return Result{ResourceID: "object-storage.s3/recovered"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	now := time.Now().UTC()
	manager.ops["op-interrupted"] = Operation{
		ID: "op-interrupted",
		Request: Request{
			Application: "demo", Capability: "object-storage.s3", Operation: "runtime.create",
			ResourceName: "recovered", IdempotencyKey: "resume",
		},
		State: StateRunning, CreatedAt: now, UpdatedAt: now,
	}
	manager.keys[idempotencyKey(manager.ops["op-interrupted"].Request)] = "op-interrupted"
	if err := manager.persistLocked(); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()

	restarted, err := New(dir, map[string]Executor{
		executorKey("object-storage.s3", "runtime.create"): ExecutorFunc(func(context.Context, Request) (Result, error) {
			return Result{ResourceID: "object-storage.s3/recovered"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForState(t, restarted, "op-interrupted", StateSucceeded)
}

func waitForState(t *testing.T, manager *Manager, id string, want State) Operation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		op, err := manager.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State == want {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	op, _ := manager.Get(id)
	t.Fatalf("operation %s state = %q, want %q", id, op.State, want)
	return Operation{}
}
