package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestApplicationStopRequiresBothCurrentAcknowledgementsWithoutReleasingBindings(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	plan, err := buildApplicationStopPlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Scope.Attention != "Achtung" || plan.Scope.ConfirmationsRequired != 2 || plan.Scope.Destructive || len(plan.Scope.Targets) != 1 || len(plan.Materials) == 0 {
		t.Fatal("stop lost concrete provider consequences or retained owners")
	}
	path := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	before, _ := os.ReadFile(path)
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: plan.Scope.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}} {
		_, err := authorizeApplicationStop(withProviderChangeConsent(t.Context(), ack), r, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatal("missing/stale stop consent bypassed shared authority", err)
		}
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	if _, err := authorizeApplicationStop(withProviderChangeConsent(context.Background(), ack), r, io.Discard); err != nil {
		t.Fatal("deliberate temporary loss permanently prohibited", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("read-only stop planning/consent released provider references")
	}
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState["recovery_reference"] = "changed-protected-reference"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeApplicationStop(withProviderChangeConsent(t.Context(), ack), r, io.Discard); err == nil {
		t.Fatal("changed actual recovery owner retained stale acknowledgement")
	}
}

func TestApplicationStopCLIJSONMCPExposeSameRequiredAcknowledgement(t *testing.T) {
	retainedNativeBackendFixture(t)
	var out bytes.Buffer
	err := runWithIO(withProviderChangeConsent(t.Context(), nil), []string{"app", "down", "--json"}, &out, io.Discard)
	var cliFailure *machine.Error
	if !errors.As(err, &cliFailure) || cliFailure.Code != machine.ErrorApprovalRequired || out.Len() != 0 {
		t.Fatalf("CLI stop consent semantics or JSON contamination: %v %s", err, out.String())
	}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "baseharbor.app.stop", Arguments: machineApplicationStopInput{}})
	if err != nil || !result.IsError {
		t.Fatalf("MCP stop bypasses required acknowledgement: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var actual machineToolError
	if err := json.Unmarshal(encoded, &actual); err != nil || actual.Error == nil || actual.Error.Code != cliFailure.Code || actual.Error.CauseCode != cliFailure.CauseCode {
		t.Fatalf("CLI/MCP error semantic drift: %s %v", encoded, err)
	}
	cliPlan, _ := json.Marshal(cliFailure.Details)
	mcpPlan, _ := json.Marshal(actual.Error.Details)
	var cliScope, mcpScope applicationStopPlan
	if json.Unmarshal(cliPlan, &cliScope) != nil || json.Unmarshal(mcpPlan, &mcpScope) != nil || cliScope.Scope.PlanID == "" || cliScope.Scope.PlanID != mcpScope.Scope.PlanID {
		t.Fatal("CLI/MCP do not expose the same concrete current stop plan")
	}
}

func TestApplicationStopHTTPUsesCurrentScopeAndCannotEscapeAuthorizedContext(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	executor := &bahaMachineExecutor{store: r.Store}
	selected := machine.OperationContext{Application: r.Manifest.Name, Environment: r.Manifest.Environment, Target: r.Target.Name}
	_, httpErr := executor.Execute(t.Context(), machine.Operation{ID: "app.stop"}, selected, json.RawMessage(`{}`), nil)
	var httpFailure *machine.Error
	if !errors.As(httpErr, &httpFailure) || httpFailure.Code != machine.ErrorApprovalRequired {
		t.Fatalf("HTTP stop did not require current impact acknowledgement: %v", httpErr)
	}
	httpPlan, _ := json.Marshal(httpFailure.Details)
	local, err := buildApplicationStopPlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded applicationStopPlan
	if json.Unmarshal(httpPlan, &decoded) != nil || decoded.Scope.PlanID != local.Scope.PlanID {
		t.Fatal("HTTP stop uses a different consequence authority")
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`{"target":"foreign"}`), json.RawMessage(`{"environment":"foreign"}`), json.RawMessage(`{"name":"foreign"}`)} {
		_, err := executor.Execute(t.Context(), machine.Operation{ID: "app.stop"}, selected, raw, nil)
		if !errors.As(err, &httpFailure) || httpFailure.Code != machine.ErrorPolicyDenied || httpFailure.CauseCode != "operation_context_mismatch" {
			t.Fatalf("HTTP stop escaped authorized selectors: %v", err)
		}
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`{"approval":true}`), json.RawMessage(`{"provider_change_consent":{"first":false,"first":true}}`)} {
		_, err := executor.Execute(t.Context(), machine.Operation{ID: "app.stop"}, selected, raw, nil)
		if !errors.As(err, &httpFailure) || httpFailure.Code != machine.ErrorValidationFailed {
			t.Fatalf("generic or ambiguous acknowledgement reached stop authority: %v", err)
		}
	}
}
