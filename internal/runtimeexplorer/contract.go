package runtimeexplorer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const ContractVersion = "baseharbor.runtime-explorer/v1"

type Ownership string

const (
	OwnershipManaged   Ownership = "managed"
	OwnershipExternal  Ownership = "external"
	OwnershipUnmanaged Ownership = "unmanaged"
	OwnershipPlatform  Ownership = "platform"
)

type ResourceKind string

const (
	KindContainer ResourceKind = "container"
	KindImage     ResourceKind = "image"
	KindVolume    ResourceKind = "volume"
	KindNetwork   ResourceKind = "network"
	KindPod       ResourceKind = "pod"
)

type ResourceRef struct {
	Provider   string       `json:"provider"`
	Target     string       `json:"target"`
	Kind       ResourceKind `json:"kind"`
	ResourceID string       `json:"resource_id"`
}

type Relationship struct {
	ApplicationID string `json:"application_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	Application   string `json:"application,omitempty"`
	Environment   string `json:"environment,omitempty"`
	Component     string `json:"component,omitempty"`
	Provider      string `json:"provider,omitempty"`
}

type ResourceState struct {
	Desired  string `json:"desired,omitempty"`
	Observed string `json:"observed,omitempty"`
	Health   string `json:"health,omitempty"`
	Ready    *bool  `json:"ready,omitempty"`
}

type ReconciliationHint struct {
	PreferredOperation            string `json:"preferred_operation,omitempty"`
	DirectMutationMayBeReconciled bool   `json:"direct_mutation_may_be_reconciled,omitempty"`
	Detail                        string `json:"detail,omitempty"`
}

type Resource struct {
	ContractVersion string             `json:"contract_version"`
	Ref             ResourceRef        `json:"ref"`
	DisplayName     string             `json:"display_name,omitempty"`
	RuntimeName     string             `json:"runtime_name,omitempty"`
	Ownership       Ownership          `json:"ownership"`
	Relationship    Relationship       `json:"relationship,omitempty"`
	State           ResourceState      `json:"state,omitempty"`
	CreatedAt       *time.Time         `json:"created_at,omitempty"`
	UpdatedAt       *time.Time         `json:"updated_at,omitempty"`
	References      map[string]string  `json:"references,omitempty"`
	Reconciliation  ReconciliationHint `json:"reconciliation,omitempty"`
	Extension       json.RawMessage    `json:"extension,omitempty"`
}

type Capability string

const (
	CapabilityResourceInspect    Capability = "resources.inspect"
	CapabilityResourceMetrics    Capability = "resources.metrics"
	CapabilityLogs               Capability = "logs"
	CapabilityContainerLifecycle Capability = "container.lifecycle"
	CapabilityContainerExec      Capability = "container.exec"
	CapabilityPodInspect         Capability = "pod.inspect"
)

type CapabilitySet struct {
	ContractVersion string         `json:"contract_version"`
	Provider        string         `json:"provider"`
	Target          string         `json:"target"`
	Capabilities    []Capability   `json:"capabilities"`
	ResourceKinds   []ResourceKind `json:"resource_kinds"`
}

type ListRequest struct {
	Target        string         `json:"target"`
	Kinds         []ResourceKind `json:"kinds,omitempty"`
	Ownership     []Ownership    `json:"ownership,omitempty"`
	ApplicationID string         `json:"application_id,omitempty"`
	DeploymentID  string         `json:"deployment_id,omitempty"`
	Environment   string         `json:"environment,omitempty"`
	Component     string         `json:"component,omitempty"`
}

type LogRequest struct {
	Resource ResourceRef `json:"resource"`
	Since    *time.Time  `json:"since,omitempty"`
	Tail     int         `json:"tail,omitempty"`
	Follow   bool        `json:"follow,omitempty"`
}

type MetricsHandle struct {
	Available bool   `json:"available"`
	Reference string `json:"reference,omitempty"`
}

type Operation string

const (
	OperationStart   Operation = "start"
	OperationStop    Operation = "stop"
	OperationRestart Operation = "restart"
	OperationExec    Operation = "exec"
)

type OperationRequest struct {
	Resource  ResourceRef `json:"resource"`
	Operation Operation   `json:"operation"`
	Command   []string    `json:"command,omitempty"`
}

type OperationResult struct {
	Resource       ResourceRef        `json:"resource"`
	Operation      Operation          `json:"operation"`
	State          ResourceState      `json:"state,omitempty"`
	Reconciliation ReconciliationHint `json:"reconciliation,omitempty"`
}

type Explorer interface {
	Capabilities(context.Context, string) (CapabilitySet, error)
	List(context.Context, ListRequest) ([]Resource, error)
	Inspect(context.Context, ResourceRef) (Resource, error)
	Logs(context.Context, LogRequest) (io.ReadCloser, error)
	Metrics(context.Context, ResourceRef) (MetricsHandle, error)
	Operate(context.Context, OperationRequest) (OperationResult, error)
}

func (r ResourceRef) Validate() error {
	if strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Target) == "" ||
		strings.TrimSpace(string(r.Kind)) == "" || strings.TrimSpace(r.ResourceID) == "" {
		return errors.New("runtime resource reference requires provider, target, kind and resource_id")
	}
	return nil
}

func (r Resource) Validate() error {
	if r.ContractVersion != ContractVersion {
		return errors.New("unsupported runtime explorer contract version")
	}
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	switch r.Ownership {
	case OwnershipManaged, OwnershipExternal, OwnershipUnmanaged, OwnershipPlatform:
	default:
		return errors.New("invalid runtime resource ownership")
	}
	if r.Ownership == OwnershipManaged &&
		strings.TrimSpace(r.Relationship.ApplicationID) == "" &&
		strings.TrimSpace(r.Relationship.DeploymentID) == "" &&
		strings.TrimSpace(r.Relationship.Provider) == "" {
		return errors.New("managed runtime resource requires BaseHarbor ownership relationship")
	}
	return nil
}

func (r OperationRequest) Validate() error {
	if err := r.Resource.Validate(); err != nil {
		return err
	}
	switch r.Operation {
	case OperationStart, OperationStop, OperationRestart:
		if len(r.Command) != 0 {
			return errors.New("runtime lifecycle operation must not carry a command")
		}
	case OperationExec:
		if len(r.Command) == 0 {
			return errors.New("runtime exec requires an explicit command")
		}
	default:
		return errors.New("unsupported runtime operation")
	}
	return nil
}
