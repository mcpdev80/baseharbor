// Command machine-control-fixtures emits synthetic public contract examples.
// It never runs a deployment or emits release evidence.
package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

func main() {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	actor := machine.ActorRef{Mode: "operator", Issuer: "https://identity.example", Subject: "synthetic-operator", Assurance: "verified", Methods: []string{"pwd"}}
	context := machine.OperationContext{Environment: "prod", Application: "synthetic-demo", Target: "synthetic-target"}
	discovery := machine.MachineDiscovery()
	discovery.HTTP = machine.MachineHTTPBindings()
	type example struct {
		Record string `json:"record"`
		Value  any    `json:"value"`
	}
	result := []example{
		{"discovery", discovery},
		{"execute_request", machinehttp.ExecuteRequest{OperationID: "status", Context: context, Input: json.RawMessage(`{}`)}},
		{"error_result", machine.ResultError(machine.NewError(machine.ErrorPolicyDenied, "Synthetic policy denial", "Review policy", false))},
	}
	for _, state := range []machine.ExecutionState{machine.ExecutionPending, machine.ExecutionRunning, machine.ExecutionSucceeded, machine.ExecutionFailed, machine.ExecutionCancelled} {
		execution := machine.Execution{ContractVersion: "v1", ExecutionID: "exec_0123456789abcdef0123456789abcdef", OperationID: "status", Actor: actor, Context: context, State: state}
		if state != machine.ExecutionPending {
			execution.StartedAt = &now
		}
		if state == machine.ExecutionSucceeded {
			execution.Result = json.RawMessage(`{"synthetic":true}`)
		}
		if state == machine.ExecutionFailed {
			execution.Error = machine.NewError(machine.ErrorRuntimeUnavailable, "Synthetic runtime unavailable", "Reconnect", true)
		}
		if state == machine.ExecutionSucceeded || state == machine.ExecutionFailed || state == machine.ExecutionCancelled {
			execution.FinishedAt = &now
		}
		result = append(result, example{"execution", execution})
	}
	for i, kind := range []machine.EventKind{machine.EventOperationStarted, machine.EventOperationProgress, machine.EventOperationSucceeded, machine.EventOperationFailed, machine.EventOperationCancelled, machine.EventApplicationState, machine.EventProviderState, machine.EventTargetState, machine.EventRuntimeResource} {
		event := machine.MachineEvent{ContractVersion: "v1", Sequence: uint64(i + 1), OccurredAt: now, Kind: kind, ExecutionID: "exec_0123456789abcdef0123456789abcdef", OperationID: "status", Actor: &actor, Context: &context}
		if kind == machine.EventOperationProgress {
			event.Progress = &machine.OperationProgress{Stage: "synthetic-progress", Current: 1, Total: 2, Percent: 50}
		}
		if kind == machine.EventOperationSucceeded {
			event.State = machine.ExecutionSucceeded
			event.Result = json.RawMessage(`{"synthetic":true}`)
		}
		if kind == machine.EventOperationFailed {
			event.State = machine.ExecutionFailed
			event.Error = machine.NewError(machine.ErrorPolicyDenied, "Synthetic denial", "Review policy", false)
		}
		result = append(result, example{"event", event})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(struct {
		Schema    string    `json:"schema"`
		Synthetic bool      `json:"synthetic"`
		Records   []example `json:"records"`
	}{"baseharbor.machine-control-fixtures/v1", true, result}); err != nil {
		panic(err)
	}
}
