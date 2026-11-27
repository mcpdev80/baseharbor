package machine

import "github.com/mcpdev80/baseharbor/internal/apierror"

// Navigation results are shared by CLI JSON, MCP and protected HTTP. Every
// list result carries the bounded pagination contract envelope as its
// `pagination` field (docs/spec/runtime-list-pagination-v1.md). A result built
// without a page request (CLI JSON, MCP, or HTTP without page parameters)
// reports the complete Core observation as one bounded page: page 1 with
// page_size equal to the observed total.
// These records describe Core observations; they do not imply live health.

// PaginateMachineList reports the complete Core observation as one bounded
// page (CLI JSON, MCP and the HTTP default
// docs/spec/runtime-list-pagination-v1.md). Callers that page a collection
// explicitly pass the validated *apierror.PageParams (nil means the full
// bounded default page).
func PaginateMachineList[T any](items []T, params *apierror.PageParams) ([]T, apierror.Pagination, *apierror.Error) {
	if items == nil {
		items = []T{}
	}
	if params == nil {
		return items, apierror.Pagination{
			Page:       apierror.FirstPage,
			PageSize:   len(items),
			Total:      len(items),
			TotalPages: apierror.FirstPage,
		}, nil
	}
	window, meta := apierror.Paginate(items, params)
	return window, meta, nil
}

type DeploymentSummary struct {
	ContractVersion string `json:"contract_version"`
	DeploymentID    string `json:"deployment_id"`
	ApplicationID   string `json:"application_id"`
	Target          string `json:"target"`
	Application     string `json:"application"`
	Environment     string `json:"environment"`
	RuntimeProvider string `json:"runtime_provider,omitempty"`
	State           string `json:"state,omitempty"`
	Ready           bool   `json:"ready"`
	SourceKind      string `json:"source_kind,omitempty"`
	SourceAvailable bool   `json:"source_available"`
}

type ApplicationListResult struct {
	Pagination      apierror.Pagination `json:"pagination"`
	ContractVersion string              `json:"contract_version"`
	Deployments     []DeploymentSummary `json:"deployments"`
	Warnings        []string            `json:"warnings,omitempty"`
}

type WorkspaceSummary struct {
	ContractVersion string            `json:"contract_version"`
	Application     string            `json:"application"`
	Manifest        string            `json:"manifest"`
	SourceCount     int               `json:"source_count"`
	Sources         map[string]string `json:"sources,omitempty"`
}

type WorkspaceListResult struct {
	Pagination      apierror.Pagination `json:"pagination"`
	ContractVersion string              `json:"contract_version"`
	Workspaces      []WorkspaceSummary  `json:"workspaces"`
	Warnings        []string            `json:"warnings,omitempty"`
}

type TargetSummary struct {
	ContractVersion string `json:"contract_version"`
	Name            string `json:"name"`
	RuntimeProvider string `json:"runtime_provider"`
	AccessReference string `json:"access_reference"`
	Scope           string `json:"scope,omitempty"`
	Implicit        bool   `json:"implicit,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Active          bool   `json:"active,omitempty"`
	Effective       bool   `json:"effective,omitempty"`
}

type TargetListResult struct {
	Pagination      apierror.Pagination `json:"pagination"`
	ContractVersion string              `json:"contract_version"`
	Targets         []TargetSummary     `json:"targets"`
}
