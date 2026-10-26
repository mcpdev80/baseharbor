package machinehttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type executorFunc func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error)

func (f executorFunc) SupportedOperationIDs() []string {
	var ids []string
	for _, operation := range machine.Operations() {
		ids = append(ids, operation.ID)
	}
	return ids
}

func (f executorFunc) Execute(ctx context.Context, op machine.Operation, opCtx machine.OperationContext, input json.RawMessage, report ProgressReporter) (json.RawMessage, error) {
	return f(ctx, op, opCtx, input, report)
}

type streamingExecutor struct {
	executorFunc
	logs string
}

func (e streamingExecutor) OpenLogStream(context.Context, machine.StreamRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(e.logs)), nil
}

func (e streamingExecutor) OpenExecStream(context.Context, machine.StreamRequest) (io.ReadWriteCloser, error) {
	return nil, machine.NewError(machine.ErrorUnsupported, "exec unavailable", "negotiate runtime capability", false)
}

func TestHandlerRequiresHTTPSAndAuthenticatedPrincipal(t *testing.T) {
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	plain := httptest.NewRequest(http.MethodGet, "/api/v1/machine/discovery", nil)
	plainRec := httptest.NewRecorder()
	handler.ServeHTTP(plainRec, plain)
	if plainRec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext status = %d", plainRec.Code)
	}

	tlsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/machine/discovery", nil)
	tlsRequest.TLS = &tls.ConnectionState{}
	tlsRec := httptest.NewRecorder()
	handler.ServeHTTP(tlsRec, tlsRequest)
	if tlsRec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous TLS status = %d", tlsRec.Code)
	}

	authRequest := withTestPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/machine/discovery", nil))
	authRec := httptest.NewRecorder()
	handler.ServeHTTP(authRec, authRequest)
	if authRec.Code != http.StatusOK {
		t.Fatalf("authenticated discovery status = %d body=%s", authRec.Code, authRec.Body.String())
	}
	if !strings.Contains(authRec.Body.String(), `"contract_version":"v1"`) {
		t.Fatalf("discovery missing machine contract: %s", authRec.Body.String())
	}
}

func TestExecutionProgressResultAndSSEAreStructuredAndSecretSafe(t *testing.T) {
	const secretMarker = "HTTP_SECRET_MUST_NOT_LEAK_91bc"
	handler, err := New(executorFunc(func(_ context.Context, op machine.Operation, opCtx machine.OperationContext, input json.RawMessage, report ProgressReporter) (json.RawMessage, error) {
		if op.ID != "status" {
			t.Fatalf("operation = %q", op.ID)
		}
		if opCtx.Environment != "prod" || opCtx.Application != "demo" || opCtx.Target != "prod-eu" {
			t.Fatalf("unexpected operation context: %#v", opCtx)
		}
		if !bytes.Contains(input, []byte(secretMarker)) {
			t.Fatal("executor did not receive bounded operation input")
		}
		report(machine.OperationProgress{Stage: "observe", Message: "Reading semantic state.", Percent: 50})
		return json.RawMessage(`{"contract_version":"v1","ready":true}`), nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{
		"operation_id":"status",
		"context":{"application":"demo","environment":"prod","target":"prod-eu"},
		"input":{"opaque":"` + secretMarker + `"}
	}`)
	discoveryRec := httptest.NewRecorder()
	handler.ServeHTTP(discoveryRec, withTestPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/machine/discovery", nil)))
	var discovery machine.Discovery
	if discoveryRec.Code != http.StatusOK || json.Unmarshal(discoveryRec.Body.Bytes(), &discovery) != nil {
		t.Fatalf("discovery failed: %s", discoveryRec.Body.String())
	}
	executeBinding, found := discovery.HTTP["execute"]
	if !found || executeBinding.Method != http.MethodPost {
		t.Fatal("discovery lacks an executable HTTP binding")
	}
	request := withTestPrincipal(httptest.NewRequest(executeBinding.Method, executeBinding.Href, body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("execute status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), secretMarker) {
		t.Fatal("operation input leaked into execution metadata")
	}

	var accepted machine.Execution
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.ExecutionID == "" || accepted.Actor.Subject != "operator-123" {
		t.Fatalf("invalid accepted execution: %#v", accepted)
	}

	var completed machine.Execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		binding := discovery.HTTP["execution"]
		get := withTestPrincipal(httptest.NewRequest(binding.Method, strings.ReplaceAll(binding.Href, "{execution_id}", accepted.ExecutionID), nil))
		getRec := httptest.NewRecorder()
		handler.ServeHTTP(getRec, get)
		if getRec.Code != http.StatusOK {
			t.Fatalf("execution status = %d body=%s", getRec.Code, getRec.Body.String())
		}
		if err := json.Unmarshal(getRec.Body.Bytes(), &completed); err != nil {
			t.Fatal(err)
		}
		if completed.State == machine.ExecutionSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed.State != machine.ExecutionSucceeded {
		t.Fatalf("execution did not complete: %#v", completed)
	}
	if completed.Progress == nil || completed.Progress.Stage != "observe" {
		t.Fatalf("progress missing: %#v", completed.Progress)
	}
	if strings.Contains(string(completed.Result), secretMarker) {
		t.Fatal("secret input leaked into result")
	}

	eventBinding := discovery.HTTP["execution_events"]
	if eventBinding.Protocol != "sse" {
		t.Fatal("discovery event binding lacks SSE protocol")
	}
	events := withTestPrincipal(httptest.NewRequest(eventBinding.Method, strings.ReplaceAll(eventBinding.Href, "{execution_id}", accepted.ExecutionID), nil))
	eventsRec := httptest.NewRecorder()
	handler.ServeHTTP(eventsRec, events)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("event stream status = %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	stream := eventsRec.Body.String()
	for _, want := range []string{"event: operation.started", "event: operation.progress", "event: operation.succeeded"} {
		if !strings.Contains(stream, want) {
			t.Fatalf("event stream missing %q: %s", want, stream)
		}
	}
	if strings.Contains(stream, secretMarker) {
		t.Fatal("secret input leaked into event stream")
	}
}

func TestProtectedLogStreamCarriesBoundedMetadata(t *testing.T) {
	const secretMarker = "LOG_SECRET_MUST_NOT_LEAK_38d1"
	handler, err := New(streamingExecutor{
		executorFunc: func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
			return nil, errors.New("not used")
		},
		logs: "line one\nline two\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{
		"contract_version":"v1",
		"context":{"application":"demo","environment":"prod","target":"prod-eu"},
		"resource_kind":"container",
		"resource_id":"runtime-resource-1"
	}`)
	request := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/streams/logs", body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("log stream status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-BaseHarbor-Resource-ID"); got != "runtime-resource-1" {
		t.Fatalf("resource id header = %q", got)
	}
	if got := recorder.Header().Get("X-BaseHarbor-Actor-Subject"); got != "operator-123" {
		t.Fatalf("actor header = %q", got)
	}
	if got := recorder.Header().Get("X-BaseHarbor-Target"); got != "prod-eu" {
		t.Fatalf("target header = %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "line one") {
		t.Fatalf("log output missing: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), secretMarker) {
		t.Fatal("unrelated secret marker leaked through stream")
	}
}

func TestExecBoundaryIsProtectedAndCapabilityGated(t *testing.T) {
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		return nil, errors.New("not used")
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{
		"contract_version":"v1",
		"context":{"environment":"prod","target":"prod-eu"},
		"resource_kind":"container",
		"resource_id":"runtime-resource-1",
		"command":["/bin/sh","-lc","echo ok"]
	}`)
	request := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/streams/exec", body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("exec capability gate status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"unsupported_operation"`) {
		t.Fatalf("exec capability denial is not typed: %s", recorder.Body.String())
	}
}

func TestExecutionCannotBeReadByDifferentOperator(t *testing.T) {
	handler, err := New(executorFunc(func(_ context.Context, _ machine.Operation, _ machine.OperationContext, _ json.RawMessage, _ ProgressReporter) (json.RawMessage, error) {
		return json.RawMessage(`{"contract_version":"v1","ok":true}`), nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"operation_id":"status","context":{"application":"demo","environment":"prod","target":"prod-eu"}}`)
	create := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", body))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, create)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var execution machine.Execution
	if err := json.Unmarshal(createRec.Body.Bytes(), &execution); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/machine/executions/"+execution.ExecutionID, nil)
	request.TLS = &tls.ConnectionState{}
	other := &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-456"}
	request = request.WithContext(identity.WithPrincipal(request.Context(), other))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-actor read status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"policy_denied"`) {
		t.Fatalf("cross-actor denial is not typed: %s", recorder.Body.String())
	}
}

func withTestPrincipal(request *http.Request) *http.Request {
	request.TLS = &tls.ConnectionState{}
	principal := &identity.Principal{
		Issuer:    "https://issuer.example",
		Subject:   "operator-123",
		Assurance: "urn:mfa",
		Methods:   []string{"pwd", "otp"},
	}
	return request.WithContext(identity.WithPrincipal(request.Context(), principal))
}
