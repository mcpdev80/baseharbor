package machinehttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestAdmissionBodyCompletesBeforeLifecycleWhileProgressRemainsObservable(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	handler, err := New(executorFunc(func(_ context.Context, _ machine.Operation, _ machine.OperationContext, _ json.RawMessage, report ProgressReporter) (json.RawMessage, error) {
		report(machine.OperationProgress{Stage: "apply", Message: "Deploying storage"})
		<-release
		return json.RawMessage(`{"verified":true}`), nil
	}), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, withTestPrincipal(r)) }))
	defer server.Close()
	defer unblock()
	client := server.Client()
	client.Timeout = 2 * time.Second
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/machine/executions", strings.NewReader(`{"operation_id":"status","context":{"environment":"prod"},"input":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Idempotency-Key", "live-progress-admission")
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("admission blocked behind lifecycle: %v", err)
	}
	defer response.Body.Close()
	var admitted machine.Execution
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&admitted); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("admission JSON body not independently complete: %v", err)
	}
	if response.StatusCode != http.StatusAccepted || response.ContentLength < 1 {
		t.Fatal("admission not framed as accepted JSON")
	}
	deadline := time.Now().Add(time.Second)
	for {
		response, err := client.Get(server.URL + "/api/v1/machine/executions/" + admitted.ExecutionID)
		if err != nil {
			t.Fatalf("progress read blocked behind admitted execution: %v", err)
		}
		var current machine.Execution
		decodeErr := json.NewDecoder(response.Body).Decode(&current)
		response.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if current.Progress != nil && current.Progress.Message == "Deploying storage" {
			if current.State != machine.ExecutionRunning {
				t.Fatal("unfinished lifecycle reported terminal")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("live service progress not observable before terminal commit")
		}
		time.Sleep(time.Millisecond)
	}
	unblock()
	waitForTerminal(t, handler, admitted.ExecutionID)
}
