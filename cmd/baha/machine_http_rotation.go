package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

// Recovery material belongs to the selected installation. HTTP cannot supply a
// host path, upload recovery keys or request a different credential authority.
type machineManagedRotationInput struct {
	Target   string `json:"target,omitempty"`
	Approval bool   `json:"approval"`
}

func bindHTTPManagedRotation(operationContext machine.OperationContext, raw json.RawMessage) (string, error) {
	var input machineManagedRotationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return "", err
	}
	target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return "", err
	}
	if target == "" || strings.TrimSpace(operationContext.Environment) == "" || operationContext.Application != "" || operationContext.Workspace != "" {
		return "", machine.NewError(machine.ErrorValidationFailed, "Select an installation target and environment for managed trust rotation.", "Use installation context without application or workspace selectors.", false)
	}
	if !input.Approval {
		return "", &machine.Error{Code: machine.ErrorPolicyDenied, CauseCode: "operator_approval_required", Message: "Managed trust rotation requires explicit operator approval.", Next: "Review the selected installation and confirm rotation."}
	}
	return target, nil
}

func executeHTTPManagedRotation(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage, report machinehttp.ProgressReporter) (any, error) {
	target, err := bindHTTPManagedRotation(operationContext, raw)
	if err != nil {
		return nil, err
	}
	ctx = machineNoninteractiveContext(withTargetOverride(ctx, target))
	if err := authorizeCurrentMCPContext(ctx, "openbao.rotate", "", "", ""); err != nil {
		return nil, err
	}
	recoveryPath, _, err := resolveTargetRecoveryFile(ctx, "")
	if err != nil || platformopenbao.ValidateRecoveryFile(recoveryPath) != nil {
		return nil, machine.NewError(machine.ErrorVerificationFailed, "Selected installation recovery material is unavailable.", "Verify the Core-owned recovery configuration locally before rotating.", false)
	}
	reportHTTPProgress(report, "rotation", "Rotating selected installation credentials and managed service trust.", 10)
	if err := rotateManagedOpenBao(ctx, recoveryPath); err != nil {
		return nil, machine.NewError(machine.ErrorVerificationFailed, "Managed trust rotation did not complete verification.", "Inspect this Core execution and installation readiness before retrying; do not replay an ambiguous rotation.", false)
	}
	result, err := inspectManagedOpenBao(ctx)
	if err != nil || !result.Initialized || !result.Unsealed || !result.ManagerReady {
		return nil, machine.NewError(machine.ErrorVerificationFailed, "Managed trust rotation readiness verification failed.", "Inspect the selected installation before continuing.", false)
	}
	reportHTTPProgress(report, "rotation", "Managed credentials and service trust rotated and verified.", 100)
	return result, nil
}
