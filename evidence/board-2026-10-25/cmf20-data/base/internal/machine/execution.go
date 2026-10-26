package machine

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const ExecutionContractVersion = "v1"

type ActorRef struct {
	Mode      string   `json:"mode"`
	Issuer    string   `json:"issuer,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Assurance string   `json:"assurance,omitempty"`
	Methods   []string `json:"authentication_methods,omitempty"`
}

type OperationContext struct {
	Application string `json:"application,omitempty"`
	Environment string `json:"environment,omitempty"`
	Target      string `json:"target,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	Resource    string `json:"resource,omitempty"`
}

type ExecutionState string

const (
	ExecutionPending   ExecutionState = "pending"
	ExecutionRunning   ExecutionState = "running"
	ExecutionSucceeded ExecutionState = "succeeded"
	ExecutionFailed    ExecutionState = "failed"
	ExecutionCancelled ExecutionState = "cancelled"
)

type OperationProgress struct {
	Stage   string  `json:"stage,omitempty"`
	Message string  `json:"message,omitempty"`
	Current int64   `json:"current,omitempty"`
	Total   int64   `json:"total,omitempty"`
	Percent float64 `json:"percent,omitempty"`
}

type Execution struct {
	ContractVersion string             `json:"contract_version"`
	ExecutionID     string             `json:"execution_id"`
	OperationID     string             `json:"operation_id"`
	Actor           ActorRef           `json:"actor"`
	Context         OperationContext   `json:"context"`
	State           ExecutionState     `json:"state"`
	StartedAt       *time.Time         `json:"started_at,omitempty"`
	FinishedAt      *time.Time         `json:"finished_at,omitempty"`
	Progress        *OperationProgress `json:"progress,omitempty"`
	Result          json.RawMessage    `json:"result,omitempty"`
	Error           *Error             `json:"error,omitempty"`
}

type EventKind string

const (
	EventOperationStarted   EventKind = "operation.started"
	EventOperationProgress  EventKind = "operation.progress"
	EventOperationSucceeded EventKind = "operation.succeeded"
	EventOperationFailed    EventKind = "operation.failed"
	EventOperationCancelled EventKind = "operation.cancelled"
	EventApplicationState   EventKind = "application.state"
	EventProviderState      EventKind = "provider.state"
	EventTargetState        EventKind = "target.state"
	EventRuntimeResource    EventKind = "runtime.resource"
)

type MachineEvent struct {
	ContractVersion string             `json:"contract_version"`
	Sequence        uint64             `json:"sequence"`
	OccurredAt      time.Time          `json:"occurred_at"`
	Kind            EventKind          `json:"kind"`
	ExecutionID     string             `json:"execution_id,omitempty"`
	OperationID     string             `json:"operation_id,omitempty"`
	Actor           *ActorRef          `json:"actor,omitempty"`
	Context         *OperationContext  `json:"context,omitempty"`
	State           ExecutionState     `json:"state,omitempty"`
	Progress        *OperationProgress `json:"progress,omitempty"`
	Result          json.RawMessage    `json:"result,omitempty"`
	Error           *Error             `json:"error,omitempty"`
	ResourceKind    string             `json:"resource_kind,omitempty"`
	ResourceID      string             `json:"resource_id,omitempty"`
}

type Discovery struct {
	HTTP             map[string]HTTPBinding `json:"http,omitempty"`
	ContractVersion  string                 `json:"contract_version"`
	ExecutionVersion string                 `json:"execution_version"`
	Operations       []Operation            `json:"operations"`
	Capabilities     []string               `json:"capabilities,omitempty"`
}

func MachineDiscovery() Discovery {
	return Discovery{
		ContractVersion:  ContractVersion,
		ExecutionVersion: ExecutionContractVersion,
		Operations:       Operations(),
		Capabilities: []string{
			"operations.discovery",
			"operations.execute",
			"operations.events",
			"streams.logs.boundary",
			"streams.exec.boundary",
		},
	}
}

func (e Execution) Validate() error {
	if e.ContractVersion != ExecutionContractVersion {
		return errors.New("unsupported execution contract version")
	}
	if strings.TrimSpace(e.ExecutionID) == "" || strings.TrimSpace(e.OperationID) == "" {
		return errors.New("execution identity is incomplete")
	}
	switch e.State {
	case ExecutionPending, ExecutionRunning, ExecutionSucceeded, ExecutionFailed, ExecutionCancelled:
	default:
		return errors.New("invalid execution state")
	}
	if e.State == ExecutionSucceeded && len(e.Result) == 0 {
		return errors.New("successful execution requires a structured result")
	}
	if e.State == ExecutionFailed && e.Error == nil {
		return errors.New("failed execution requires a structured error")
	}
	return nil
}
