package repositoryinspect

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectionMachineContractOmitsComposeSpecificIdentity(t *testing.T) {
	result := Result{
		ContractVersion:          "v1",
		ComposeCandidates:        []string{"compose.yaml"},
		SelectedCompose:          "compose.yaml",
		WorkloadSourceCandidates: []WorkloadSourceCandidate{{Kind: WorkloadSourceCompose, Path: "compose.yaml"}},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "compose_candidates") || strings.Contains(text, "selected_compose") {
		t.Fatalf("Compose-specific identity leaked into machine contract: %s", text)
	}
	if !strings.Contains(text, "workload_source_candidates") {
		t.Fatalf("source-neutral candidates missing from machine contract: %s", text)
	}
	if !strings.Contains(text, "workload_source_resolution") {
		t.Fatalf("standardized resolution missing from machine contract: %s", text)
	}
}
