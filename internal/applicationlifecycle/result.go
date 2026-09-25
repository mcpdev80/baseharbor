package applicationlifecycle

import "github.com/mcpdev80/baseharbor/internal/machine"

type Result struct {
	ContractVersion string `json:"contract_version"`
	Operation       string `json:"operation"`
	Target          string `json:"target,omitempty"`
	Application     string `json:"application,omitempty"`
	Environment     string `json:"environment,omitempty"`
	State           string `json:"state"`
	Verified        bool   `json:"verified"`
	Detail          string `json:"detail,omitempty"`
}

func NewResult(operation, application, environment, state string, verified bool) Result {
	return Result{
		ContractVersion: machine.ContractVersion,
		Operation:       operation,
		Application:     application,
		Environment:     environment,
		State:           state,
		Verified:        verified,
	}
}

func (r Result) WithTarget(target string) Result {
	r.Target = target
	return r
}

func RequireApproval(operation string, approved bool) error {
	if approved {
		return nil
	}
	return &machine.Error{
		Code:        machine.ErrorApprovalRequired,
		CauseCode:   "explicit_approval_required",
		Message:     "Explicit approval is required before " + operation + ".",
		Remediation: "requires operator approval",
		Next:        "Retry the operation with approval=true after reviewing the destructive or risk-bearing action.",
	}
}
