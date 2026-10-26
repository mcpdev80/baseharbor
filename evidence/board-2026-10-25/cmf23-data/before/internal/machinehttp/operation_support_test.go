package machinehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type selectedExecutor struct {
	executorFunc
	ids []string
}

func (e selectedExecutor) SupportedOperationIDs() []string { return e.ids }

func TestDiscoveryAndDispatchShareExactHTTPExecutionSupport(t *testing.T) {
	calls := 0
	executor := selectedExecutor{ids: []string{"status"}, executorFunc: func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{}`), nil
	}}
	handler, err := New(executor)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withTestPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/machine/discovery", nil)))
	var discovery machine.Discovery
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &discovery) != nil || len(discovery.Operations) != 1 || discovery.Operations[0].ID != "status" {
		t.Fatal("discovery differs from executable support", response.Body.String())
	}
	canonical, _ := machine.OperationByID("status")
	if discovery.Operations[0] != canonical {
		t.Fatal("transport changed canonical safety or authorization metadata")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, withTestPrincipal(httptest.NewRequest(http.MethodPost, discovery.HTTP["execute"].Href, strings.NewReader(`{"operation_id":"operator.identity","context":{"environment":"prod"}}`))))
	if response.Code != http.StatusNotFound || calls != 0 {
		t.Fatal("unimplemented transport operation accepted", response.Code, calls)
	}
	for _, ids := range [][]string{{"not.registered"}, {"status", "status"}} {
		if _, err := New(selectedExecutor{ids: ids, executorFunc: executor.executorFunc}); err == nil {
			t.Fatal("invalid support registry accepted")
		}
	}
}
