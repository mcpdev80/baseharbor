package machinehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestViewerCannotAdmitMachineMutation(t *testing.T) {
	for _, operation := range []string{"apply", "destroy"} {
		t.Run(operation, func(t *testing.T) {
			called := make(chan struct{}, 1)
			handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
				called <- struct{}{}
				return json.RawMessage("{}"), nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			request := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions",
				strings.NewReader(`{"operation_id":"`+operation+`","context":{"environment":"dev","target":"local"}}`)))
			request = request.WithContext(tenancy.WithContext(request.Context(), &tenancy.Context{
				TenantID: "tenant-a", ExternalIdentityID: "identity-a", Roles: []string{"viewer"},
			}))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"cause":"tenant_permission_denied"`) {
				t.Fatalf("viewer admission status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Header().Get("Location") != "" {
				t.Fatal("denied operation acquired an execution identity")
			}
			select {
			case <-called:
				t.Fatal("denied operation reached the executor")
			default:
			}
		})
	}
}
