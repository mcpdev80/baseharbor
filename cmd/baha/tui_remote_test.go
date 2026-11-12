package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestTUIRemoteReadsOwningCoreWithoutLocalDeployment(t *testing.T) {
	ctx, target, core := remoteJourneyFixture(t)
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-tui-docker.sock")
	core.readResults = map[string]json.RawMessage{
		"status": json.RawMessage(`{"contract_version":"v1","target":"local","application":"demo","environment":"dev","state":"running","ready":false}`),
		"doctor": json.RawMessage(`{"contract_version":"v1","target":"local","application":"demo","environment":"dev","healthy":false,"checks":[{"name":"workload","ok":false,"detail":"not ready"}]}`),
	}
	m := tuiModel{ctx: ctx, store: application.Store{Root: t.TempDir()}}
	msg := m.loadStatus()().(tuiStatusMsg)
	if msg.err != nil || msg.result.Target != target.Name || msg.result.Application != "demo" || msg.result.Ready || msg.doctor.Healthy || len(msg.doctor.Checks) != 1 {
		t.Fatalf("remote degraded observations lost: %+v", msg)
	}
	if strings.Join(core.operations, ",") != "status,doctor" {
		t.Fatalf("unexpected operations: %v", core.operations)
	}
	// Refresh must use the same read-only authority, without local cached state.
	msg = m.loadStatus()().(tuiStatusMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
}

func TestTUIRemoteRejectsForeignApplicationResponses(t *testing.T) {
	for _, operation := range []string{"status", "doctor"} {
		t.Run(operation, func(t *testing.T) {
			ctx, _, core := remoteJourneyFixture(t)
			core.readResults = map[string]json.RawMessage{
				"status": json.RawMessage(`{"contract_version":"v1","target":"local","application":"demo","environment":"dev"}`),
				"doctor": json.RawMessage(`{"contract_version":"v1","target":"local","application":"demo","environment":"dev"}`),
			}
			core.readResults[operation] = json.RawMessage(`{"contract_version":"v1","target":"local","application":"foreign","environment":"dev"}`)
			msg := (tuiModel{ctx: ctx}).loadStatus()().(tuiStatusMsg)
			if msg.err == nil || !strings.Contains(msg.err.Error(), "authority") {
				t.Fatalf("foreign observation accepted: %+v", msg)
			}
		})
	}
}

func TestTUIRemoteCoreAndJSONAvoidWorkstationRuntime(t *testing.T) {
	ctx, target, core := remoteJourneyFixture(t)
	t.Chdir(t.TempDir())
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-tui-docker.sock")
	core.readResults = map[string]json.RawMessage{
		"control-plane.status": json.RawMessage(`{"target":"local","state":"running","ready":true,"running":["postgres"],"checks":[{"name":"postgres","ready":true}]}`),
		"control-plane.doctor": json.RawMessage(`{"ready":true,"core_version":"dev","checks":[{"name":"postgres","ready":true}]}`),
	}
	msg := (tuiModel{ctx: ctx, coreMode: true}).loadStatus()().(tuiStatusMsg)
	if msg.err != nil || !strings.Contains(msg.coreView, target.Name) || !strings.Contains(msg.coreDoctor, "postgres") {
		t.Fatalf("remote Core views failed: %+v", msg)
	}
	status, err := inspectTUIControlPlane(ctx)
	if err != nil || status.Target != target.Name {
		t.Fatalf("JSON authority mismatch: %+v %v", status, err)
	}
}
