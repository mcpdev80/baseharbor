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
}

type ErrorCode string

const (
	ErrorValidationFailed    ErrorCode = "validation_failed"
	ErrorPolicyDenied        ErrorCode = "policy_denied"
	ErrorConflict            ErrorCode = "conflict"
	ErrorOwnershipAmbiguous  ErrorCode = "ownership_ambiguous"
	ErrorProviderUnavailable ErrorCode = "provider_unavailable"
	ErrorRuntimeUnavailable  ErrorCode = "runtime_unavailable"
	ErrorTimeout             ErrorCode = "timeout"
	ErrorVerificationFailed  ErrorCode = "verification_failed"
	ErrorUnsupported         ErrorCode = "unsupported_operation"
	ErrorInternal            ErrorCode = "internal_error"
)

type Error struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable,omitempty"`
	Next      string    `json:"next,omitempty"`
	Cause     error     `json:"-"`
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
		{ID: "inspect", Description: "Inspect repository evidence without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "plan", Description: "Build the deterministic desired-state plan without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "status", Description: "Observe application runtime and readiness state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "doctor", Description: "Run diagnostic verification without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "policy.check", Description: "Evaluate effective environment policy without mutation.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "policy.explain", Description: "Explain effective policy defaults and bounded overrides.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
	}
}

func CapabilitySpecifications() []string {
	return []string{
		"cache.key-value",
		"database.sql",
		"exposure.http/v1",
		"logs/v1",
		"metrics/v1",
		"object-storage.s3/v1",
		"secure-binding/v1",
		"telemetry.otlp/v1",
		"traces/v1",
	}
}
