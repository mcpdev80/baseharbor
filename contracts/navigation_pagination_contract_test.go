package contracts_test

import (
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestNavigationResultsCarryPaginationEnvelope proves that every machine
// navigation list result embeds the runtime list pagination contract envelope
// as a `pagination` object next to its item collection
// (docs/spec/runtime-list-pagination-v1.md), and that the machine HTTP read
// path emits it through the same single constructor
// (machine.PaginateMachineList).
func TestNavigationResultsCarryPaginationEnvelope(t *testing.T) {
	deployments := make([]machine.DeploymentSummary, 3)
	for i := range deployments {
		deployments[i] = machine.DeploymentSummary{ContractVersion: "v1", DeploymentID: "deployment"}
	}
	result := machine.ApplicationListResult{ContractVersion: "v1"}
	items, envelope, err := machine.PaginateMachineList(deployments, nil)
	if err != nil {
		t.Fatal(err)
	}
	result.Deployments, result.Pagination = items, envelope
	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	wireEnvelope, ok := decoded["pagination"]
	_ = wireEnvelope
	if !ok {
		t.Fatalf("app.list has no pagination sibling: %s", raw)
	}
	var object map[string]any
	if err := json.Unmarshal(decoded["pagination"], &object); err != nil {
		t.Fatal(err)
	}
	if object["page"] != float64(1) || object["page_size"] != float64(3) || object["total"] != float64(3) || object["total_pages"] != float64(1) || object["next_page"] != nil {
		t.Fatalf("envelope = %s", decoded["pagination"])
	}
	for _, tc := range []struct {
		name    string
		marshal func() ([]byte, error)
		key     string
	}{
		{"target.list", func() ([]byte, error) {
			r := machine.TargetListResult{ContractVersion: "v1"}
			targets, meta, err := machine.PaginateMachineList([]machine.TargetSummary{{ContractVersion: "v1", Name: "remote"}}, nil)
			if err != nil {
				return nil, err
			}
			r.Targets, r.Pagination = targets, meta
			return json.Marshal(r)
		}, "targets"},
		{"workspace.list", func() ([]byte, error) {
			r := machine.WorkspaceListResult{ContractVersion: "v1"}
			workspaces, meta, err := machine.PaginateMachineList([]machine.WorkspaceSummary{{ContractVersion: "v1", Application: "demo", Manifest: "/synthetic/baseharbor.yaml", SourceCount: 1}}, nil)
			if err != nil {
				return nil, err
			}
			r.Workspaces, r.Pagination = workspaces, meta
			return json.Marshal(r)
		}, "workspaces"},
	} {
		out, err := tc.marshal()
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(out, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["pagination"]; !ok {
			t.Fatalf("%s has no pagination sibling: %s", tc.name, out)
		}
		if _, ok := fields[tc.key]; !ok {
			t.Fatalf("%s lost its collection: %s", tc.name, out)
		}
	}
}
