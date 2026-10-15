package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func nativeTargetDefinitionRecovery(t *testing.T, ctx context.Context, session *mcp.ClientSession, provider bhruntime.RuntimeProvider, project, name string, verifyOriginal func()) {
	t.Helper()
	observe := func() map[string]bhruntime.RuntimeContainer {
		t.Helper()
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal("native Target resource inspection", err)
		}
		result := map[string]bhruntime.RuntimeContainer{}
		for _, container := range containers {
			if container.Project == project {
				container.Health, container.State, container.Error = "", "", ""
				result[container.ID] = container
			}
		}
		return result
	}
	before := observe()
	resources, err := provider.ListOwnedProjectResources(ctx, project)
	if err != nil || len(before) < 3 {
		t.Fatal("original live Core/sibling inventory unavailable", err)
	}
	var out bytes.Buffer
	if err := runWithIO(withProviderChangeConsent(ctx, nil), []string{"target", "delete", name, "-o", "json"}, &out, io.Discard); err == nil || machine.Classify(err).Code != machine.ErrorApprovalRequired {
		t.Fatal("native unacknowledged Target detach accepted", err)
	}
	plan, err := buildTargetDefinitionChangePlan(ctx, name, "")
	if err != nil {
		t.Fatal(err)
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "baseharbor.target.delete", Arguments: machineTargetDeleteInput{Name: name, ProviderChangeConsent: ack}})
	if err != nil || response.IsError {
		t.Fatalf("native MCP Target detach: %#v %v", response, err)
	}
	raw, _ := json.Marshal(response.StructuredContent)
	var result targetMutationResult
	if json.Unmarshal(raw, &result) != nil || !result.Deleted || result.RecoveryFile == "" || result.ProviderChangeResult == nil || result.ProviderChangeResult.Status != "partial" {
		t.Fatal("Target detach emitted false actual outcome")
	}
	if _, err := effectiveTarget(ctx); err == nil || machine.Classify(err).CauseCode != "target_selection_detached" {
		t.Fatal("Target detach silently selected alternate runtime/authentication", err)
	}
	if !reflect.DeepEqual(before, observe()) {
		t.Fatal("Target detach restarted/removed original Core or running sibling")
	}
	current, err := provider.ListOwnedProjectResources(ctx, project)
	if err != nil || !reflect.DeepEqual(resources, current) {
		t.Fatal("Target detach changed original volumes/networks/process owners", err)
	}
	verifyOriginal()
	restore, err := buildTargetDefinitionChangePlan(ctx, name, result.RecoveryFile)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	ack = &application.ProviderChangeConsent{PlanID: restore.Scope.PlanID, First: true, Second: true}
	if err := runWithIO(withProviderChangeConsent(ctx, ack), []string{"target", "create", name, "--from-recovery", result.RecoveryFile, "-o", "json"}, &out, io.Discard); err != nil {
		t.Fatal("native CLI original-definition restoration", err)
	}
	if _, err := effectiveTarget(ctx); err == nil {
		t.Fatal("Target restoration silently activated its access route")
	}
	if !reflect.DeepEqual(before, observe()) {
		t.Fatal("Target restoration rematerialized original native processes")
	}
	if err := writePersistedTarget(name); err != nil {
		t.Fatal(err)
	}
	if selected, err := effectiveTarget(ctx); err != nil || selected.Name != name {
		t.Fatal("explicit original Target activation failed", err)
	}
	verifyOriginal()
	t.Log("native MCP Target definition detach and CLI protected original-definition restore verified; unchanged original process IDs, volumes/networks, running sibling, SQL/TLS data and original OpenBao credentials; no reseeding, rotation or automatic access fallback; all-consumer receipt remains partial")
}
