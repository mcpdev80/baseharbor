package machinehttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestBrowserOriginAndExpiredPrincipalFailBeforeExecution(t *testing.T) {
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		t.Error("invalid browser request reached executor")
		return nil, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"http://core.example", "https://foreign.example", "null", "https://core.example/path", "https://user:secret@core.example", "https://core.example?token=secret"} {
		request := withTestPrincipal(httptest.NewRequest("GET", "https://core.example/api/v1/machine/discovery", nil))
		request.Header.Set("Origin", origin)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || strings.Contains(recorder.Body.String(), origin) {
			t.Fatal("foreign/ambiguous browser origin accepted or echoed")
		}
	}
	request := withTestPrincipal(httptest.NewRequest("GET", "https://core.example/api/v1/machine/discovery", nil))
	request.Header.Set("Origin", "https://core.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatal("same-origin discovery denied")
	}
	expiry := time.Now().Add(-time.Second)
	principal, _ := identity.FromContext(request.Context())
	principal.ExpiresAt = &expiry
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatal("expired principal accepted")
	}
}

type pendingLogExecutor struct {
	executorFunc
	opened chan context.Context
}

func (e pendingLogExecutor) OpenLogStream(ctx context.Context, _ machine.StreamRequest) (io.ReadCloser, error) {
	reader, _ := io.Pipe()
	e.opened <- ctx
	return reader, nil
}

func TestOpenLogStreamTerminatesAtTokenExpiryAndWithoutProducerOutput(t *testing.T) {
	opened := make(chan context.Context, 1)
	handler, err := New(pendingLogExecutor{opened: opened})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"contract_version":"v1","context":{"environment":"dev"},"resource_kind":"container","resource_id":"owned","follow":true}`
	request := withTestPrincipal(httptest.NewRequest("POST", "https://core.example/api/v1/machine/streams/logs", strings.NewReader(body)))
	expiry := time.Now().Add(100 * time.Millisecond)
	principal, _ := identity.FromContext(request.Context())
	principal.ExpiresAt = &expiry
	finished := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
		close(finished)
	}()
	select {
	case ctx := <-opened:
		deadline, ok := ctx.Deadline()
		if !ok || !deadline.Equal(expiry) {
			t.Fatal("stream lifetime is not bound to token expiry")
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not open")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expired idle stream remained open")
	}
}

func TestDeadlineWriterRejectsOutputAfterSessionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	if _, err := (streamDeadlineWriter{ResponseWriter: recorder, ctx: ctx}).Write([]byte("must not appear")); err == nil || recorder.Body.Len() != 0 {
		t.Fatal("cancelled stream delivered output")
	}
}
