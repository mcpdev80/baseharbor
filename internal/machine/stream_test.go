package machine

import "testing"

func TestStreamContractRequiresExplicitResourceAndExecCommand(t *testing.T) {
	logs := StreamRequest{
		ContractVersion: StreamContractVersion,
		Kind:            StreamLogs,
		ResourceKind:    "container",
		ResourceID:      "runtime-resource-1",
	}
	if err := logs.Validate(); err != nil {
		t.Fatalf("valid log stream rejected: %v", err)
	}

	exec := StreamRequest{
		ContractVersion: StreamContractVersion,
		Kind:            StreamExec,
		ResourceKind:    "container",
		ResourceID:      "runtime-resource-1",
		Command:         []string{"/bin/sh", "-lc", "echo ok"},
	}
	if err := exec.Validate(); err != nil {
		t.Fatalf("valid exec stream rejected: %v", err)
	}

	exec.Command = nil
	if err := exec.Validate(); err == nil {
		t.Fatal("exec without explicit command unexpectedly accepted")
	}
}
