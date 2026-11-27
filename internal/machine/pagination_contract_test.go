package machine_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestNavigationPaginationContract proves the runtime list pagination contract
// (docs/spec/runtime-list-pagination-v1.md) on the machine navigation results
// shared by CLI JSON, MCP and protected HTTP: every list result serializes a
// `pagination` envelope as a sibling of its item collection, a result built
// without a page request answers the complete observation as one bounded page,
// and invalid page parameters fail closed.
func TestNavigationPaginationContract(t *testing.T) {
	deployments := make([]machine.DeploymentSummary, 130)
	for i := range deployments {
		deployments[i] = machine.DeploymentSummary{ContractVersion: "v1", DeploymentID: "deployment"}
	}
	full, envelope, err := machine.PaginateMachineList(deployments, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 130 || envelope.Page != 1 || envelope.PageSize != 130 || envelope.Total != 130 || envelope.TotalPages != 1 || envelope.NextPage != nil {
		t.Fatalf("default full page: %d %+v", len(full), envelope)
	}
	params, invalid := apierror.ParsePageParams(url.Values{"page": []string{"2"}, "page_size": []string{"100"}})
	if invalid != nil {
		t.Fatal(invalid)
	}
	window, envelope, err := machine.PaginateMachineList(deployments, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 30 || envelope.Page != 2 || envelope.TotalPages != 2 || envelope.NextPage != nil {
		t.Fatalf("windowed page two: %d %+v", len(window), envelope)
	}
	if _, invalid := apierror.ParsePageParams(url.Values{"page": []string{"0"}}); invalid == nil || invalid.Code != apierror.CodeBadRequest {
		t.Fatalf("invalid page must fail closed: %v", invalid)
	}
	if _, invalid := apierror.ParsePageParams(url.Values{"page_size": []string{"101"}}); invalid == nil || invalid.Code != apierror.CodeBadRequest {
		t.Fatalf("invalid page_size must fail closed: %v", invalid)
	}
	result := machine.ApplicationListResult{ContractVersion: "v1"}
	fullItems, fullEnvelope, envelopeErr := machine.PaginateMachineList(deployments, nil)
	if envelopeErr != nil {
		t.Fatal(envelopeErr)
	}
	result.Deployments, result.Pagination = fullItems, fullEnvelope
	raw, marshalErr := json.Marshal(&result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["pagination"]; !ok {
		t.Fatalf("app.list result has no pagination sibling: %s", raw)
	}
	if _, ok := decoded["deployments"]; !ok {
		t.Fatalf("app.list result lost its collection: %s", raw)
	}
}
