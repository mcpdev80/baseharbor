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

func (allowAuthorizer) AuthorizeRuntimeOperation(string, string, string, string) error { return nil }

type denyAuthorizer struct{}

func (denyAuthorizer) AuthorizeRuntimeOperation(string, string, string, string) error {
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

func TestCrossApplicationResourceAccessFailsClosed(t *testing.T) {
	executorCalls := 0
	executor := runtimeoperation.ExecutorFunc(func(_ context.Context, request runtimeoperation.Request) (runtimeoperation.Result, error) {
		executorCalls++
		return runtimeoperation.Result{
			ResourceID: "res-s3-owned-by-alpha",
			Binding: map[string]any{
				"endpoint":          "http://seaweedfs:8333",
				"bucket":            "alpha-bucket",
				"access_key_id":     "scoped-access",
				"secret_access_key": "scoped-secret",
			},
		}, nil
	})
	manager, err := runtimeoperation.New(t.TempDir(), map[string]runtimeoperation.Executor{
		"object-storage.s3/v1\x00runtime.create": executor,
		"object-storage.s3/v1\x00runtime.delete": executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := New("alpha", manager, allowAuthorizer{}, executor)
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", strings.NewReader(`{"capability":"object-storage.s3/v1","name":"assets"}`))
	create.Header.Set("Idempotency-Key", "alpha-assets")
	createRes := httptest.NewRecorder()
	alpha.ServeHTTP(createRes, create)
	if createRes.Code != http.StatusAccepted {
		t.Fatalf("alpha create status = %d body=%s", createRes.Code, createRes.Body.String())
	}
	var accepted map[string]any
	if err := json.Unmarshal(createRes.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	operationID, _ := accepted["id"].(string)
	waitHTTPState(t, alpha, operationID, "succeeded")

	beta, err := New("beta", manager, allowAuthorizer{}, executor)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/runtime/v1/resources/res-s3-owned-by-alpha"},
		{method: http.MethodGet, path: "/runtime/v1/resources/res-s3-owned-by-alpha/binding"},
		{method: http.MethodDelete, path: "/runtime/v1/resources/res-s3-owned-by-alpha"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.method == http.MethodDelete {
			req.Header.Set("Idempotency-Key", "cross-app-delete")
		}
		res := httptest.NewRecorder()
		beta.ServeHTTP(res, req)
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d body=%s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}

	if executorCalls != 1 {
		t.Fatalf("cross-application access reached executor: calls=%d want=1", executorCalls)
	}
}

type serviceAuthorizer struct{}

func (serviceAuthorizer) AuthorizeRuntimeOperation(app, service, capability, operation string) error {
	if app != "demo" || service != "api" || capability != "metrics/v1" || operation != "runtime.create" {
		return errors.New("denied")
	}
	return nil
}

func TestRuntimeResourceAuthorizationIsServiceScoped(t *testing.T) {
	manager, err := runtimeoperation.New(t.TempDir(), map[string]runtimeoperation.Executor{
		"metrics/v1\x00runtime.create": runtimeoperation.ExecutorFunc(func(context.Context, runtimeoperation.Request) (runtimeoperation.Result, error) {
			return runtimeoperation.Result{ResourceID: "metrics/application"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New("demo", manager, serviceAuthorizer{}, testExecutor)
	if err != nil {
		t.Fatal(err)
	}

	makeRequest := func(service string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/runtime/v1/resources", strings.NewReader(`{"capability":"metrics/v1","name":"application"}`))
		req.Header.Set("Idempotency-Key", "metrics-"+service)
		req = WithRuntimeService(req, service)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	if res := makeRequest("worker"); res.Code != http.StatusForbidden {
		t.Fatalf("worker status = %d body=%s", res.Code, res.Body.String())
	}
	res := makeRequest("api")
	if res.Code != http.StatusAccepted {
		t.Fatalf("api status = %d body=%s", res.Code, res.Body.String())
	}
	var accepted map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	operationID, _ := accepted["id"].(string)
	if operationID == "" {
		t.Fatalf("operation id missing: %s", res.Body.String())
	}
	waitHTTPState(t, h, operationID, "succeeded")
}
