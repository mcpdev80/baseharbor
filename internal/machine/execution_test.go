package machine

import (
	"encoding/json"
	"testing"
)

func TestMachineDiscoveryAndExecutionContract(t *testing.T) {
	discovery := MachineDiscovery()
	if discovery.ContractVersion != ContractVersion || discovery.ExecutionVersion != ExecutionContractVersion {
		t.Fatalf("unexpected discovery versions: %#v", discovery)
	}
	if len(discovery.Operations) != len(Operations()) {
		t.Fatalf("operation count = %d, want %d", len(discovery.Operations), len(Operations()))
	}

	result, err := json.Marshal(map[string]any{"ready": true})
	if err != nil {
		t.Fatal(err)
	}
	execution := Execution{
		ContractVersion: ExecutionContractVersion,
		ExecutionID:     "exec-1",
		OperationID:     "status",
		State:           ExecutionSucceeded,
		Result:          result,
	}
	if err := execution.Validate(); err != nil {
		t.Fatalf("valid execution rejected: %v", err)
	}

	execution.State = ExecutionFailed
	execution.Result = nil
	execution.Error = NewError(ErrorPolicyDenied, "denied", "review policy", false)
	if err := execution.Validate(); err != nil {
		t.Fatalf("valid failed execution rejected: %v", err)
	}
}
