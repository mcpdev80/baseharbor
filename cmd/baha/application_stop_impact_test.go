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
