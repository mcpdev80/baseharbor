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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type executorFunc func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error)

func TestWriteJSONReturnsValidErrorWhenResponseCannotBeEncoded(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, struct {
		Unsupported any `json:"unsupported"`
	}{make(chan int)})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var result machine.ErrorResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("fallback response is invalid JSON: %v", err)
	}
	if result.Error == nil || result.Error.Code != machine.ErrorInternal {
		t.Fatalf("fallback error = %#v, want internal_error", result.Error)
	}
}

// hasTerminal reports whether history already carries a terminal event
// (succeeded/failed). The terminal event is emitted synchronously by
// succeed()/fail() before runExecution returns, so a synchronous candidate has
// it in history; a backgrounded mutant does not.
func hasTerminal(history []machine.MachineEvent) bool {
	for _, ev := range history {
		if ev.Kind == machine.EventOperationSucceeded || ev.Kind == machine.EventOperationFailed {
			return true
		}
	}
	return false
}

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
	}), "")
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
	}), "")
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
	request.Header.Set("Idempotency-Key", "handler-success")
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
	acceptedExecutionID := accepted.ExecutionID
	history, eventsCh, cancel, ok := handler.executions.subscribe(acceptedExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	// The terminal state is committed on the 202's tail, so a terminal event is
	// already present in history; a backgrounded mutant would not signal it
	// synchronously and this wait would time out. No poll, no sleep.
	if !hasTerminal(history) {
		select {
		case ev := <-eventsCh:
			if ev.Kind != machine.EventOperationSucceeded && ev.Kind != machine.EventOperationFailed {
				t.Fatal("unexpected non-terminal event on 202 tail")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("terminal event not recorded synchronously on 202 tail")
		}
	}
	// Fetch the completed execution to assert on its fields.
	execBinding := discovery.HTTP["execution"]
	completedReq := withTestPrincipal(httptest.NewRequest(execBinding.Method, strings.ReplaceAll(execBinding.Href, "{execution_id}", acceptedExecutionID), nil))
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, completedReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("execution status = %d body=%s", getRec.Code, getRec.Body.String())
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
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
	}, "")

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
	}), "")
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
	}), "")
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"operation_id":"status","context":{"application":"demo","environment":"prod","target":"prod-eu"}}`)
	create := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", body))
	create.Header.Set("Idempotency-Key", "handler-cross-actor")
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

func TestInstallationDestroyReturnsTerminalReceiptAfterRemovingJournal(t *testing.T) {
	stateRoot := t.TempDir()
	calls := 0
	handler, err := New(executorFunc(func(_ context.Context, op machine.Operation, scope machine.OperationContext, _ json.RawMessage, _ ProgressReporter) (json.RawMessage, error) {
		calls++
		if op.ID != "installation.destroy" || scope.Target != "local" || scope.Environment != "dev" {
			t.Fatalf("unexpected destroy request: op=%q scope=%#v", op.ID, scope)
		}
		if err := os.RemoveAll(stateRoot); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"verified":true}`), nil
	}), stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"operation_id":"installation.destroy","context":{"environment":"dev","target":"local"},"input":{"approval":true},"wait_for_terminal":true}`
	standard := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", strings.NewReader(strings.Replace(body, `,"wait_for_terminal":true`, "", 1))))
	standard.Header.Set("Idempotency-Key", "destroy-without-terminal-mode")
	standard = standard.WithContext(tenancy.WithContext(standard.Context(), &tenancy.Context{
		TenantID: "tenant-a", ExternalIdentityID: "identity-a", Roles: []string{"editor"},
	}))
	standardResponse := httptest.NewRecorder()
	handler.ServeHTTP(standardResponse, standard)
	if standardResponse.Code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("nonterminal installation destroy status=%d calls=%d body=%s", standardResponse.Code, calls, standardResponse.Body.String())
	}
	request := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", strings.NewReader(body)))
	request.Header.Set("Idempotency-Key", "installation-destroy-terminal-receipt")
	request = request.WithContext(tenancy.WithContext(request.Context(), &tenancy.Context{
		TenantID: "tenant-a", ExternalIdentityID: "identity-a", Roles: []string{"editor"},
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("destroy status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var execution machine.Execution
	if err := json.Unmarshal(recorder.Body.Bytes(), &execution); err != nil {
		t.Fatal(err)
	}
	if execution.OperationID != "installation.destroy" || execution.State != machine.ExecutionSucceeded || string(execution.Result) != `{"verified":true}` {
		t.Fatalf("terminal receipt=%#v", execution)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destroy journal root survived: %v", err)
	}
	replay := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", strings.NewReader(body)))
	replay.Header.Set("Idempotency-Key", "installation-destroy-terminal-receipt")
	replay = replay.WithContext(tenancy.WithContext(replay.Context(), &tenancy.Context{
		TenantID: "tenant-a", ExternalIdentityID: "identity-a", Roles: []string{"editor"},
	}))
	replayResponse := httptest.NewRecorder()
	handler.ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusOK || calls != 1 {
		t.Fatalf("idempotent terminal replay status=%d calls=%d body=%s", replayResponse.Code, calls, replayResponse.Body.String())
	}
	var replayed machine.Execution
	if err := json.Unmarshal(replayResponse.Body.Bytes(), &replayed); err != nil || replayed.State != machine.ExecutionSucceeded {
		t.Fatalf("terminal replay = %#v err=%v", replayed, err)
	}
}

func withTestPrincipal(request *http.Request) *http.Request {
	request.TLS = &tls.ConnectionState{}
	principal := &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "operator-123",
		Assurance:      "urn:mfa",
		Methods:        []string{"pwd", "otp"},
		IssuerVerified: true,
		Groups:         []string{"editor"},
	}
	// Mirror the production path: the principal must be visible via
	// identity.FromContext (discovery/handler principal checks) and, because the
	// production boundary now requires a verified principal with a resolved claim
	// membership, registered in the operatorauth enforcement context with a
	// bounded provider.
	ctx := identity.WithPrincipal(request.Context(), principal)
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	ctx = operatorauth.WithMembershipProvider(ctx, operatorauthClaimStub{})
	return request.WithContext(ctx)
}

// operatorauthClaimStub is the minimal bounded provider binding these machinehttp
// tests need: it maps the verified principal onto a single-tenant editor
// membership so the claim-to-membership boundary admits it. It lives here because
// the production path now requires a verified principal with a resolved claim
// membership; the shared operatorauth test stub is not exported.
type operatorauthClaimStub struct{}

func (operatorauthClaimStub) ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if principal == nil || !principal.IssuerVerified {
		return nil, operatorauth.ErrClaimsNotVerified
	}
	return []tenancy.Membership{{TenantID: "tenant-a", Role: "editor"}}, nil
}
