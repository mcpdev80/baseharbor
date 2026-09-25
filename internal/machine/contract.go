package machine

import (
	"context"
	"errors"
	"fmt"
)

const ContractVersion = "v1"

type SafetyClass string

const (
	SafetyReadOnly    SafetyClass = "read_only"
	SafetyMutating    SafetyClass = "mutating"
	SafetyDestructive SafetyClass = "destructive"
)

type Operation struct {
	ID                   string      `json:"id"`
	Description          string      `json:"description"`
	Safety               SafetyClass `json:"safety"`
	ConfirmationRequired bool        `json:"confirmation_required"`
	PolicyRequired       bool        `json:"policy_required"`
	ContractVersion      string      `json:"contract_version"`
	MCPTool              string      `json:"mcp_tool,omitempty"`
}

type ErrorCode string

const (
	ErrorValidationFailed      ErrorCode = "validation_failed"
	ErrorPortConflict          ErrorCode = "port_conflict"
	ErrorRequiredSecretMissing ErrorCode = "required_secret_missing"
	ErrorSourceMissing         ErrorCode = "source_missing"
	ErrorApprovalRequired      ErrorCode = "approval_required"
	ErrorWorkloadStartFailed   ErrorCode = "workload_start_failed"
	ErrorImagePullFailed       ErrorCode = "image_pull_failed"
	ErrorAuthenticationFailed  ErrorCode = "authentication_failed"
	ErrorInvalidWorkload       ErrorCode = "invalid_workload"
	ErrorCapabilityMissing     ErrorCode = "capability_missing"
	ErrorPolicyDenied          ErrorCode = "policy_denied"
	ErrorConflict              ErrorCode = "conflict"
	ErrorOwnershipAmbiguous    ErrorCode = "ownership_ambiguous"
	ErrorProviderUnavailable   ErrorCode = "provider_unavailable"
	ErrorRuntimeUnavailable    ErrorCode = "runtime_unavailable"
	ErrorTimeout               ErrorCode = "timeout"
	ErrorVerificationFailed    ErrorCode = "verification_failed"
	ErrorUnsupported           ErrorCode = "unsupported_operation"
	ErrorInternal              ErrorCode = "internal_error"
)

type Error struct {
	Code        ErrorCode `json:"code"`
	Message     string    `json:"message"`
	CauseCode   string    `json:"cause,omitempty"`
	Retryable   bool      `json:"retryable,omitempty"`
	Next        string    `json:"next,omitempty"`
	Resource    string    `json:"resource,omitempty"`
	Remediation string    `json:"remediation_class,omitempty"`
	Cause       error     `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewError(code ErrorCode, message, next string, retryable bool) *Error {
	if message == "" {
		message = string(code)
	}
	return &Error{Code: code, Message: message, Next: next, Retryable: retryable}
}

func Wrap(code ErrorCode, err error, next string, retryable bool) *Error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Message: err.Error(), Next: next, Retryable: retryable, Cause: err}
}

func Classify(err error) *Error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Wrap(ErrorTimeout, err, "Retry after confirming the local runtime and providers are responsive.", true)
	case errors.Is(err, context.Canceled):
		return Wrap(ErrorInternal, err, "Retry the operation if it was cancelled unintentionally.", true)
	default:
		return Wrap(ErrorInternal, fmt.Errorf("%w", err), "Inspect the error and run baha doctor for additional diagnostics.", false)
	}
}

type ErrorResult struct {
	ContractVersion string `json:"contract_version"`
	Error           *Error `json:"error"`
}

func ResultError(err error) ErrorResult {
	return ErrorResult{ContractVersion: ContractVersion, Error: Classify(err)}
}

func Operations() []Operation {
	return []Operation{
		{ID: "target", MCPTool: "baseharbor.target", Description: "Inspect the effective BaseHarbor deployment target and repository-resolved identity.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "inspect", MCPTool: "baseharbor.inspect", Description: "Inspect repository evidence without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "plan", MCPTool: "baseharbor.plan", Description: "Build the deterministic desired-state plan without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "apply", MCPTool: "baseharbor.apply", Description: "Converge and verify the selected application.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "status", MCPTool: "baseharbor.status", Description: "Observe application runtime and readiness state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "doctor", MCPTool: "baseharbor.doctor", Description: "Run diagnostic verification without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "observe", MCPTool: "baseharbor.observe", Description: "Return secret-safe application diagnostics and observability state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "update", MCPTool: "baseharbor.update", Description: "Safely update repository source and reconverge the application.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "repair", MCPTool: "baseharbor.repair", Description: "Repair safely reconcilable BaseHarbor-owned application drift and verify the result.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "backup", MCPTool: "baseharbor.backup", Description: "Create and verify the currently supported encrypted application recovery unit.", Safety: SafetyMutating, ContractVersion: ContractVersion},
		{ID: "restore", MCPTool: "baseharbor.restore", Description: "Restore and verify the currently supported encrypted application recovery unit.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "destroy", MCPTool: "baseharbor.destroy", Description: "Permanently remove BaseHarbor-owned application runtime resources and state.", Safety: SafetyDestructive, ConfirmationRequired: true, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "policy.check", MCPTool: "baseharbor.policy.check", Description: "Evaluate effective environment policy without mutation.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "policy.explain", MCPTool: "baseharbor.policy.explain", Description: "Explain effective policy defaults and bounded overrides.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
	}
}

func MCPTools() []string {
	operations := Operations()
	tools := make([]string, 0, len(operations))
	for _, operation := range operations {
		if operation.MCPTool != "" {
			tools = append(tools, operation.MCPTool)
		}
	}
	return tools
}

func OperationByID(id string) (Operation, bool) {
	for _, operation := range Operations() {
		if operation.ID == id {
			return operation, true
		}
	}
	return Operation{}, false
}

func CapabilitySpecifications() []string {
	return []string{
		"cache.key-value/v1",
		"database.sql/v1",
		"exposure.http/v1",
		"logs/v1",
		"metrics/v1",
		"object-storage.s3/v1",
		"secure-binding/v1",
		"telemetry.otlp/v1",
		"traces/v1",
	}
}
