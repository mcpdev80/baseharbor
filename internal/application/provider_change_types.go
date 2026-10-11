package application

import "github.com/mcpdev80/baseharbor/internal/capability"

const ProviderImpactVersion = "baseharbor.provider-impact/v1"

// These are read-only projections of existing owners, not a new state store.
// Only authorized references cross this boundary. Never supply secret contents.
type ResourceLocation struct {
	Owner     string `json:"owner"`
	Target    string `json:"target"`
	Kind      string `json:"kind"` // volume, path, external-store, backup, recovery
	Reference string `json:"reference"`
}

type ProviderAccess struct {
	Method   string                   `json:"method"`
	Endpoint string                   `json:"endpoint,omitempty"`
	Security capability.SecureBinding `json:"security"`
}

type ProviderRealization struct {
	Ancillary  bool                         `json:"ancillary,omitempty"`
	InstanceID string                       `json:"instance_id"`
	Kind       capability.ProviderKind      `json:"kind"`
	Target     string                       `json:"target"`
	Scope      capability.ProviderScope     `json:"scope"`
	Ownership  capability.ProviderOwnership `json:"ownership"`
	Owner      string                       `json:"owner"`
	State      ProviderState                `json:"state"`
	Locations  []ResourceLocation           `json:"locations"`
	Access     *ProviderAccess              `json:"access,omitempty"`
	Supported  bool                         `json:"supported"`
	Verified   bool                         `json:"verified"`
	Evidence   string                       `json:"evidence,omitempty"`
}

type ProviderMaterial struct {
	Retention *MaterialRetention           `json:"retention,omitempty"`
	Ownership capability.ProviderOwnership `json:"ownership"`
	ID        string                       `json:"id"`
	Kind      string                       `json:"kind"`
	Location  *ResourceLocation            `json:"location,omitempty"`
	Recovery  []ResourceLocation           `json:"recovery"`
	Access    *ProviderAccess              `json:"access,omitempty"`
	ExpiresAt string                       `json:"expires_at,omitempty"`
	Verified  bool                         `json:"verified"`
	Evidence  string                       `json:"evidence,omitempty"`
}

// Function use is actual observed usage. PKI, dynamic leases, cached token
// validation and sessions are never inferred merely from a provider product.
type ProviderFunctionUse struct {
	Consumer   string `json:"consumer"`
	InstanceID string `json:"instance_id"`
	Function   string `json:"function"`
	Required   bool   `json:"required"`
	ValidUntil string `json:"valid_until,omitempty"`
	Behavior   string `json:"behavior,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}

type OperationalConsequence struct {
	Aspect   string `json:"aspect"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Evidence string `json:"evidence,omitempty"`
}

type ProviderPrerequisite struct {
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Supported   bool   `json:"supported"`
	Verified    bool   `json:"verified"`
	Evidence    string `json:"evidence,omitempty"`
	Consequence string `json:"consequence"`
}

type ProviderImpactFact struct {
	Prerequisites     []ProviderPrerequisite   `json:"prerequisites"`
	Realization       ProviderRealization      `json:"realization"`
	Materials         []ProviderMaterial       `json:"materials"`
	InventoryComplete bool                     `json:"inventory_complete"`
	Effects           []OperationalConsequence `json:"effects"`
}

type ProviderFunctionContinuation struct {
	Consumer  string `json:"consumer"`
	Function  string `json:"function"`
	From      string `json:"from"`
	To        string `json:"to"`
	Supported bool   `json:"supported"`
	Verified  bool   `json:"verified"`
	Evidence  string `json:"evidence,omitempty"`
}

type ProviderImpactObservation struct {
	ObservedAt        string                         `json:"observed_at,omitempty"`
	ConsumerActions   []ConsumerTransition           `json:"consumer_actions,omitempty"`
	Continuations     []ProviderFunctionContinuation `json:"continuations"`
	Revision          string                         `json:"revision"`
	InventoryComplete bool                           `json:"inventory_complete"`
	Providers         map[string]ProviderImpactFact  `json:"providers"`
	Functions         []ProviderFunctionUse          `json:"functions"`
}

// Transfers describe an existing supported adapter operation. Supported and
// verified refer to actual handoff/access checks, never a proposed fallback.
type MaterialDisposition struct {
	MaterialID  string            `json:"material_id"`
	Disposition string            `json:"disposition"`
	Destination *ResourceLocation `json:"destination,omitempty"`
	AccessAfter *ProviderAccess   `json:"access_after,omitempty"`
	Supported   bool              `json:"supported"`
	Verified    bool              `json:"verified"`
	Evidence    string            `json:"evidence,omitempty"`
}

type ProviderChangeRequest struct {
	ConsumerActions []string              `json:"consumer_actions,omitempty"`
	InstanceID      string                `json:"instance_id"`
	Operation       string                `json:"operation"`
	ReplacementID   string                `json:"replacement_id,omitempty"`
	Selection       string                `json:"selection,omitempty"` // automatic or explicit
	Consumers       []string              `json:"consumers"`           // empty means all users
	Materials       []MaterialDisposition `json:"materials"`
}

type FunctionConsequence struct {
	Transition               *ConsumerTransition `json:"transition,omitempty"`
	RemainingValiditySeconds *int64              `json:"remaining_validity_seconds,omitempty"`
	Consumer                 string              `json:"consumer"`
	Function                 string              `json:"function"`
	Via                      []string            `json:"via"`
	Required                 bool                `json:"required"`
	Outcome                  string              `json:"outcome"` // remains, changed, unavailable, unknown
	ValidUntil               string              `json:"valid_until,omitempty"`
	Behavior                 string              `json:"behavior,omitempty"`
	Evidence                 string              `json:"evidence,omitempty"`
}

type MaterialConsequence struct {
	RemainingValiditySeconds *int64              `json:"remaining_validity_seconds,omitempty"`
	Before                   ProviderMaterial    `json:"before"`
	After                    MaterialDisposition `json:"after"`
	Meaning                  string              `json:"meaning"`
}

type ProviderImpact struct {
	Prerequisites      []ProviderPrerequisite   `json:"prerequisites"`
	RequestedConsumers []string                 `json:"requested_consumers"`
	InstanceID         string                   `json:"instance_id"`
	Operation          string                   `json:"operation"`
	Scenario           string                   `json:"scenario"`
	Selection          string                   `json:"selection"`
	Before             ProviderRealization      `json:"before"`
	After              *ProviderRealization     `json:"after,omitempty"`
	Dependencies       []string                 `json:"dependencies"`
	Dependents         []string                 `json:"dependents"`
	Functions          []FunctionConsequence    `json:"functions"`
	Materials          []MaterialConsequence    `json:"materials"`
	Effects            []OperationalConsequence `json:"effects"`
	Notices            []string                 `json:"notices"`
}

type ProviderImpactPlan struct {
	Version               string           `json:"version"`
	Target                string           `json:"target"`
	Revision              string           `json:"revision"`
	PlanID                string           `json:"plan_id"`
	Complete              bool             `json:"complete"` // knowledge completeness, NOT a policy veto
	Attention             string           `json:"attention,omitempty"`
	ConfirmationsRequired int              `json:"confirmations_required"`
	Destructive           bool             `json:"destructive"`
	Impacts               []ProviderImpact `json:"impacts"`
}

// Each stage is an independently explicit acknowledgement of the SAME plan.
// Existing interfaces collect these; this adds no flags, prompts or state owner.
type ProviderChangeConsent struct {
	PlanID     string `json:"plan_id"`
	First      bool   `json:"first"`
	Second     bool   `json:"second"`
	DeleteData bool   `json:"delete_data"`
}

type MaterialOutcome struct {
	MaterialID  string             `json:"material_id"`
	Disposition string             `json:"disposition"`
	Location    *ResourceLocation  `json:"location,omitempty"`
	Recovery    []ResourceLocation `json:"recovery"`
	Access      *ProviderAccess    `json:"access,omitempty"`
	Verified    bool               `json:"verified"`
	Evidence    string             `json:"evidence,omitempty"`
}

type ProviderFunctionOutcome struct {
	Transition *ConsumerTransition `json:"transition,omitempty"`
	Consumer   string              `json:"consumer"`
	InstanceID string              `json:"instance_id"`
	Function   string              `json:"function"`
	Outcome    string              `json:"outcome"`
	ValidUntil string              `json:"valid_until,omitempty"`
	Behavior   string              `json:"behavior,omitempty"`
	Verified   bool                `json:"verified"`
	Evidence   string              `json:"evidence,omitempty"`
}

type ProviderExecutionOutcome struct {
	Effects    []OperationalConsequence  `json:"effects,omitempty"`
	Functions  []ProviderFunctionOutcome `json:"functions,omitempty"`
	Verified   bool                      `json:"verified"`
	Evidence   string                    `json:"evidence,omitempty"`
	InstanceID string                    `json:"instance_id"`
	Operation  string                    `json:"operation"`
	Status     string                    `json:"status"` // applied, failed, pending
	After      *ProviderRealization      `json:"after,omitempty"`
	Materials  []MaterialOutcome         `json:"materials"`
	ErrorCode  string                    `json:"error_code,omitempty"`
}

type ProviderChangeResult struct {
	Version  string                     `json:"version"`
	PlanID   string                     `json:"plan_id"`
	Status   string                     `json:"status"` // complete, partial, failed
	Outcomes []ProviderExecutionOutcome `json:"outcomes"`
}
