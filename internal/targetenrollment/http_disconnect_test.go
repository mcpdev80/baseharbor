package targetenrollment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func disconnectHTTPRequest(path, body, role string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://core.example"+path, strings.NewReader(body))
	return request.WithContext(grantHTTPRequest(role).Context())
}
func TestDisconnectHTTPRequiresCommonAuthorityBeforeMutation(t *testing.T) {
	handler, _ := enrollmentHTTPFixture(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, disconnectHTTPRequest(DisconnectPath, `{"target_id":"lab","node_id":"node-a","environment":"dev"}`, "editor"))
	if response.Code != http.StatusNotImplemented {
		t.Fatal("unconfigured direct revocation fallback", response.Code)
	}
}
func TestDisconnectPlanNeverForwardsConsentAndAllRoutesAuthorize(t *testing.T) {
	handler, _ := enrollmentHTTPFixture(t)
	calls := 0
	handler.disconnect = func(_ context.Context, scope Scope, raw json.RawMessage) (any, error) {
		calls++
		if scope != testScope() || len(raw) != 0 {
			t.Fatal("read-only route forwarded approval or wrong scope")
		}
		return nil, machine.NewError(machine.ErrorApprovalRequired, "concrete plan required", "Review both explicit stages.", false)
	}
	body := `{"target_id":"lab","node_id":"node-a","environment":"dev","provider_change_consent":{"plan_id":"approved","first":true,"second":true}}`
	for _, path := range []string{DisconnectPlanPath, DisconnectPath} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, disconnectHTTPRequest(path, body, "viewer"))
		if response.Code != http.StatusForbidden || calls != 0 {
			t.Fatal("unprivileged disconnect authority invoked", response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, disconnectHTTPRequest(DisconnectPlanPath, body, "editor"))
	if response.Code != http.StatusConflict || calls != 1 || !strings.Contains(response.Body.String(), string(machine.ErrorApprovalRequired)) {
		t.Fatal("plan route executed or suppressed concrete denial", response.Code)
	}
}
func TestDisconnectHTTPRejectsAmbiguousConsentBeforeAuthority(t *testing.T) {
	handler, _ := enrollmentHTTPFixture(t)
	calls := 0
	handler.disconnect = func(context.Context, Scope, json.RawMessage) (any, error) { calls++; return nil, nil }
	body := `{"target_id":"lab","node_id":"node-a","environment":"dev","provider_change_consent":{"first":false,"first":true}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, disconnectHTTPRequest(DisconnectPath, body, "editor"))
	if response.Code != http.StatusBadRequest || calls != 0 {
		t.Fatal("ambiguous nested consent reached authority")
	}
}
