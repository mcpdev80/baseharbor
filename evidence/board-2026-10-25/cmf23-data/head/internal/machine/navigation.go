package machine

// Navigation results are shared by CLI JSON, MCP and protected HTTP.
// These records describe Core observations; they do not imply live health.
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
	ContractVersion string             `json:"contract_version"`
	Workspaces      []WorkspaceSummary `json:"workspaces"`
	Warnings        []string           `json:"warnings,omitempty"`
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
	ContractVersion string          `json:"contract_version"`
	Targets         []TargetSummary `json:"targets"`
}
