package contracts_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

func TestMachineControlGoldenRecordsMatchCoreDiscovery(t *testing.T) {
	raw, err := contracts.ReadMachineControlGoldenFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Synthetic bool `json:"synthetic"`
		Records   []struct {
			Record string          `json:"record"`
			Value  json.RawMessage `json:"value"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &examples); err != nil || !examples.Synthetic || len(examples.Records) != 17 {
		t.Fatal("invalid synthetic fixtures", err)
	}
	for _, example := range examples.Records {
		if err := contracts.ValidateMachineControlRecord(example.Record, example.Value); err != nil {
			t.Fatalf("%s: %v", example.Record, err)
		}
		if example.Record == "discovery" {
			var actual machine.Discovery
			if err := json.Unmarshal(example.Value, &actual); err != nil {
				t.Fatal(err)
			}
			expected := machine.MachineDiscovery()
			expected.HTTP = machine.MachineHTTPBindings()
			if !reflect.DeepEqual(actual, expected) {
				t.Fatal("public discovery fixture differs from Core operation/binding registry")
			}
		}
	}
}

func TestMachineControlSchemasAcceptActualCoreRecords(t *testing.T) {
	now := time.Now().UTC()
	discovery := machine.MachineDiscovery()
	discovery.HTTP = machine.MachineHTTPBindings()
	actor := machine.ActorRef{Mode: "operator", Issuer: "https://identity.example", Subject: "operator", Assurance: "verified", Methods: []string{"pwd"}}
	context := machine.OperationContext{Environment: "prod", Application: "demo", Target: "lab"}
	execution := machine.Execution{ContractVersion: "v1", ExecutionID: "exec_0123456789abcdef0123456789abcdef", OperationID: "status", Actor: actor, Context: context, State: machine.ExecutionSucceeded, StartedAt: &now, FinishedAt: &now, Result: json.RawMessage(`{}`), Progress: &machine.OperationProgress{Stage: "complete", Current: 1, Total: 1, Percent: 100}}
	cases := map[string]any{
		"discovery": discovery, "execution": execution,
		"execute_request": machinehttp.ExecuteRequest{OperationID: "status", Context: context, Input: json.RawMessage(`{}`)},
		"event":           machine.MachineEvent{ContractVersion: "v1", Sequence: 1, OccurredAt: now, Kind: machine.EventOperationSucceeded, ExecutionID: execution.ExecutionID, OperationID: execution.OperationID, Actor: &actor, Context: &context, State: execution.State, Result: execution.Result},
		"error_result":    machine.ResultError(machine.NewError(machine.ErrorPolicyDenied, "Denied", "Review policy", false)),
	}
	for record, value := range cases {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := contracts.ValidateMachineControlRecord(record, data); err != nil {
			t.Fatalf("%s: %v", record, err)
		}
	}
	execution.State = machine.ExecutionFailed
	execution.Result = nil
	execution.Error = machine.NewError(machine.ErrorRuntimeUnavailable, "Unavailable", "Reconnect", true)
	data, _ := json.Marshal(execution)
	if err := contracts.ValidateMachineControlRecord("execution", data); err != nil {
		t.Fatal(err)
	}
}

func TestMachineControlSchemasRejectAmbiguousAndIncompleteRecords(t *testing.T) {
	for _, test := range []struct{ record, data string }{
		{"execute_request", `{"operation_id":"status","context":{}}`},
		{"execute_request", `{"operation_id":"status","context":{"environment":"prod"},"input":[]}`},
		{"execute_request", `{"operation_id":"status","operation_id":"app.destroy","context":{"environment":"prod"}}`},
		{"event", `{"contract_version":"v1","sequence":0,"occurred_at":"2026-10-06T00:00:00Z","kind":"operation.started"}`},
		{"event", `{"contract_version":"v2","sequence":1,"occurred_at":"2026-10-06T00:00:00Z","kind":"operation.started"}`},
		{"error_result", `{"contract_version":"v1","error":{"code":"x"}}`},
		{"error_result", `{"contract_version":"v1","error":{"code":"x","message":"y"},"unknown":true}`},
		{"error_result", `{"contract_version":"v1","error":{"code":"x","message":"y"}} {}`},
	} {
		if contracts.ValidateMachineControlRecord(test.record, []byte(test.data)) == nil {
			t.Fatal("invalid record accepted", test.record)
		}
	}
}
