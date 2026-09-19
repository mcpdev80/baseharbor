package runtimeresourceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

type allowAuthorizer struct{}

func (allowAuthorizer) AuthorizeRuntimeOperation(string, string, string) error { return nil }

type denyAuthorizer struct{}

func (denyAuthorizer) AuthorizeRuntimeOperation(string, string, string) error {
	return errors.New("denied")
}

var testExecutor = runtimeoperation.ExecutorFunc(func(context.Context, runtimeoperation.Request) (runtimeoperation.Result, error) {
	return runtimeoperation.Result{}, nil
})

func TestAsyncCreateAndOperationStatus(t *testing.T) {
	release := make(chan struct{})
	manager, err := runtimeoperation.New(t.TempDir(), map[string]runtimeoperation.Executor{
		"object-storage.s3\x00runtime.create": runtimeoperation.ExecutorFunc(func(ctx context.Context, request runtimeoperation.Request) (runtimeoperation.Result, error) {
			select {
			case <-release:
				return runtimeoperation.Result{ResourceID: "object-storage.s3/" + request.ResourceName}, nil
			case <-ctx.Done():
				return runtimeoperation.Result{}, ctx.Err()
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New("demo", manager, allowAuthorizer{}, testExecutor)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", strings.NewReader(`{"capability":"object-storage.s3","name":"user-4711"}`))
	req.Header.Set("Idempotency-Key", "tenant-4711-storage")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", res.Code, res.Body.String())
	}
	var accepted map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	id, _ := accepted["id"].(string)
	if id == "" {
		t.Fatalf("operation id missing: %#v", accepted)
	}

	waitHTTPState(t, h, id, "running")
	close(release)
	waitHTTPState(t, h, id, "succeeded")

	replay := httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", strings.NewReader(`{"capability":"object-storage.s3","name":"user-4711"}`))
	replay.Header.Set("Idempotency-Key", "tenant-4711-storage")
	replayRes := httptest.NewRecorder()
	h.ServeHTTP(replayRes, replay)
	if replayRes.Code != http.StatusOK || !strings.Contains(replayRes.Body.String(), id) {
		t.Fatalf("idempotent replay = %d %s", replayRes.Code, replayRes.Body.String())
	}
}

func TestCreateRequiresAuthorizationAndIdempotency(t *testing.T) {
	manager, err := runtimeoperation.New(t.TempDir(), map[string]runtimeoperation.Executor{
		"object-storage.s3\x00runtime.create": runtimeoperation.ExecutorFunc(func(context.Context, runtimeoperation.Request) (runtimeoperation.Result, error) {
			return runtimeoperation.Result{}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := New("demo", manager, denyAuthorizer{}, testExecutor)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", bytes.NewBufferString(`{"capability":"object-storage.s3","name":"tenant"}`))
	req.Header.Set("Idempotency-Key", "key")
	res := httptest.NewRecorder()
	denied.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d", res.Code)
	}

	allowed, err := New("demo", manager, allowAuthorizer{}, testExecutor)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", bytes.NewBufferString(`{"capability":"object-storage.s3","name":"tenant"}`))
	res = httptest.NewRecorder()
	allowed.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d", res.Code)
	}
}

func waitHTTPState(t *testing.T, h http.Handler, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/runtime/v1/operations/"+id, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("operation status = %d body=%s", res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), `"state":"`+want+`"`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("operation %s did not reach %s", id, want)
}
