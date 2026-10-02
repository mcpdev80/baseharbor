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
	ErrorValidationFailed         ErrorCode = "validation_failed"
	ErrorPortConflict             ErrorCode = "port_conflict"
	ErrorRequiredSecretMissing    ErrorCode = "required_secret_missing"
	ErrorSourceMissing            ErrorCode = "source_missing"
	ErrorApprovalRequired         ErrorCode = "approval_required"
	ErrorWorkloadStartFailed      ErrorCode = "workload_start_failed"
	ErrorImagePullFailed          ErrorCode = "image_pull_failed"
	ErrorAuthenticationFailed     ErrorCode = "authentication_failed"
	ErrorInvalidWorkload          ErrorCode = "invalid_workload"
	ErrorCapabilityMissing        ErrorCode = "capability_missing"
	ErrorPolicyDenied             ErrorCode = "policy_denied"
	ErrorConflict                 ErrorCode = "conflict"
	ErrorOwnershipAmbiguous       ErrorCode = "ownership_ambiguous"
	ErrorProviderUnavailable      ErrorCode = "provider_unavailable"
	ErrorRuntimeUnavailable       ErrorCode = "runtime_unavailable"
	ErrorHostResourceInsufficient ErrorCode = "host_resource_insufficient"
	ErrorTimeout                  ErrorCode = "timeout"
	ErrorVerificationFailed       ErrorCode = "verification_failed"
	ErrorUnsupported              ErrorCode = "unsupported_operation"
	ErrorInternal                 ErrorCode = "internal_error"
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
		{ID: "target.list", MCPTool: "baseharbor.target.list", Description: "List configured BaseHarbor deployment targets using secret-safe target metadata.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "runtime.capabilities", MCPTool: "baseharbor.runtime.capabilities", Description: "Inspect Runtime Explorer capabilities for the effective Target runtime provider.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.list", MCPTool: "baseharbor.runtime.list", Description: "List provider-neutral runtime resources with authoritative ownership and BaseHarbor relationships.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.inspect", MCPTool: "baseharbor.runtime.inspect", Description: "Inspect one stable runtime resource reference without mutation.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.operate", MCPTool: "baseharbor.runtime.operate", Description: "Perform a bounded low-level lifecycle action on an authorized Runtime Explorer resource.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.start", MCPTool: "baseharbor.runtime.start", Description: "Start one authorized concrete runtime resource through Runtime Explorer.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.stop", MCPTool: "baseharbor.runtime.stop", Description: "Stop one authorized concrete runtime resource through Runtime Explorer.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "runtime.restart", MCPTool: "baseharbor.runtime.restart", Description: "Restart one authorized concrete runtime resource through Runtime Explorer.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "app.list", MCPTool: "baseharbor.app.list", Description: "List registered application deployments using stable identity and secret-safe observed state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "inspect", MCPTool: "baseharbor.inspect", Description: "Inspect repository evidence without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "workspace.list", MCPTool: "baseharbor.workspace.list", Description: "List developer-local workspace mappings without mutating source or runtime state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "workspace.resolve", MCPTool: "baseharbor.workspace.resolve", Description: "Resolve canonical component/source identity to developer-local worktrees without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "workspace.status", MCPTool: "baseharbor.workspace.status", Description: "Inspect Git state for mapped repository sources without changing checked-out revisions.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "workspace.update", MCPTool: "baseharbor.workspace.update", Description: "Safely fetch and fast-forward mapped repository sources when Git state is unambiguous.", Safety: SafetyMutating, ContractVersion: ContractVersion},
		{ID: "app.new", MCPTool: "baseharbor.app.new", Description: "Create and validate a new ecosystem-native application from portable capability intent.", Safety: SafetyMutating, ContractVersion: ContractVersion},
		{ID: "plan", MCPTool: "baseharbor.plan", Description: "Build the deterministic desired-state plan without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "apply", MCPTool: "baseharbor.apply", Description: "Converge and verify the selected application.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "status", MCPTool: "baseharbor.status", Description: "Observe application runtime and readiness state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "doctor", MCPTool: "baseharbor.doctor", Description: "Run diagnostic verification without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "observe", MCPTool: "baseharbor.observe", Description: "Return secret-safe application diagnostics and observability state.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "evidence", MCPTool: "baseharbor.evidence", Description: "Export deterministic secret-safe lifecycle, policy, verification and recovery evidence.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "update", MCPTool: "baseharbor.update", Description: "Safely update repository source and reconverge the application.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "repair", MCPTool: "baseharbor.repair", Description: "Repair safely reconcilable BaseHarbor-owned application drift and verify the result.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "backup", MCPTool: "baseharbor.backup", Description: "Create and verify the currently supported encrypted application recovery unit.", Safety: SafetyMutating, ContractVersion: ContractVersion},
		{ID: "restore", MCPTool: "baseharbor.restore", Description: "Restore and verify the currently supported encrypted application recovery unit.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "destroy", MCPTool: "baseharbor.destroy", Description: "Permanently remove BaseHarbor-owned application runtime resources and state.", Safety: SafetyDestructive, ConfirmationRequired: true, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "policy.check", MCPTool: "baseharbor.policy.check", Description: "Evaluate effective environment policy without mutation.", Safety: SafetyReadOnly, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "provider.list", MCPTool: "baseharbor.provider.list", Description: "List registered externally owned capability providers.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "provider.inspect", MCPTool: "baseharbor.provider.inspect", Description: "Inspect one registered external capability provider.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "provider.verify", MCPTool: "baseharbor.provider.verify", Description: "Verify external provider reachability and configured trust without mutation.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "provider.add", MCPTool: "baseharbor.provider.add", Description: "Register externally owned provider deployment state and references.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "provider.remove", MCPTool: "baseharbor.provider.remove", Description: "Remove BaseHarbor external-provider registration without mutating foreign infrastructure.", Safety: SafetyDestructive, ConfirmationRequired: true, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "organization.inspect", MCPTool: "baseharbor.organization.inspect", Description: "Inspect active organization/platform configuration and effective defaults.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "organization.check", MCPTool: "baseharbor.organization.check", Description: "Resolve the configured organization source without changing active configuration.", Safety: SafetyReadOnly, ContractVersion: ContractVersion},
		{ID: "organization.set", MCPTool: "baseharbor.organization.set", Description: "Resolve and activate an organization/platform configuration source.", Safety: SafetyMutating, PolicyRequired: true, ContractVersion: ContractVersion},
		{ID: "organization.update", MCPTool: "baseharbor.organization.update", Description: "Explicitly activate the configured organization source at its newly resolved immutable version.", Safety: SafetyMutating, ConfirmationRequired: true, PolicyRequired: true, ContractVersion: ContractVersion},
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
		"database.document/v1",
		"database.key-value/v1",
		"database.sql/v1",
		"exposure.http/v1",
		"identity.oidc/v1",
		"logs/v1",
		"messaging.pubsub/v1",
		"messaging.queue/v1",
		"messaging.stream/v1",
		"metrics/v1",
		"object-storage.s3/v1",
		"secrets/v1",
		"secure-binding/v1",
		"telemetry.otlp/v1",
		"traces/v1",
	}
}
