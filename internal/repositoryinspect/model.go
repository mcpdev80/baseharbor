package repositoryinspect

import (
	"context"
)

type Confidence string

const (
	ConfidenceDetected  Confidence = "detected"
	ConfidenceSuggested Confidence = "suggested"
	ConfidencePossible  Confidence = "possible"
)

type Direction string

const (
	DirectionConsume   Direction = "consume"
	DirectionProvide   Direction = "provide"
	DirectionExport    Direction = "export"
	DirectionReceive   Direction = "receive"
	DirectionProvision Direction = "provision"
)

type RuntimeOperation string

const (
	RuntimeCreate RuntimeOperation = "runtime.create"
	RuntimeGet    RuntimeOperation = "runtime.get"
	RuntimeDelete RuntimeOperation = "runtime.delete"
	RuntimeRotate RuntimeOperation = "runtime.rotate"
)

type ReconciliationState string

const (
	ReconciliationSatisfied ReconciliationState = "satisfied"
	ReconciliationNew       ReconciliationState = "new"
	ReconciliationStale     ReconciliationState = "stale"
	ReconciliationAmbiguous ReconciliationState = "ambiguous"
)

type EvidenceKind string

const (
	EvidenceManifest   EvidenceKind = "manifest"
	EvidenceCompose    EvidenceKind = "compose"
	EvidenceDependency EvidenceKind = "dependency"
	EvidenceEnv        EvidenceKind = "env"
	EvidenceImport     EvidenceKind = "import"
	EvidencePort       EvidenceKind = "port"
	EvidenceHealth     EvidenceKind = "healthcheck"
	EvidenceConfig     EvidenceKind = "config"
	EvidenceEndpoint   EvidenceKind = "endpoint"
	EvidenceCall       EvidenceKind = "call"
)

type Evidence struct {
	Kind   EvidenceKind `json:"kind"`
	Path   string       `json:"path"`
	Detail string       `json:"detail"`
}

type Finding struct {
	Service    string             `json:"service,omitempty"`
	Protocol   string             `json:"protocol,omitempty"`
	Capability string             `json:"capability"`
	Name       string             `json:"name,omitempty"`
	Direction  Direction          `json:"direction,omitempty"`
	Operations []RuntimeOperation `json:"operations,omitempty"`
	Confidence Confidence         `json:"confidence"`
	Evidence   []Evidence         `json:"evidence"`
}

type CapabilityIntent struct {
	Capability string    `json:"capability"`
	Name       string    `json:"name,omitempty"`
	Direction  Direction `json:"direction,omitempty"`
}

type ReconciliationItem struct {
	Capability string              `json:"capability"`
	Name       string              `json:"name,omitempty"`
	Direction  Direction           `json:"direction,omitempty"`
	State      ReconciliationState `json:"state"`
	Operations []RuntimeOperation  `json:"operations,omitempty"`
	Confidence Confidence          `json:"confidence,omitempty"`
	Evidence   []Evidence          `json:"evidence,omitempty"`
}

type Artifact struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type PortEvidence struct {
	Path    string `json:"path"`
	Service string `json:"service,omitempty"`
	Value   string `json:"value"`
}

type Result struct {
	ContractVersion        string               `json:"contract_version"`
	Root                   string               `json:"root"`
	Application            string               `json:"application"`
	ExistingManifest       string               `json:"existing_manifest,omitempty"`
	Artifacts              []Artifact           `json:"artifacts,omitempty"`
	ComposeCandidates      []string             `json:"compose_candidates,omitempty"`
	SelectedCompose        string               `json:"selected_compose,omitempty"`
	WorkloadServices       []string             `json:"workload_services,omitempty"`
	InfrastructureServices []string             `json:"infrastructure_services,omitempty"`
	AmbiguousServices      []string             `json:"ambiguous_services,omitempty"`
	Findings               []Finding            `json:"findings,omitempty"`
	RequiredSecrets        []string             `json:"required_secrets,omitempty"`
	SecretCandidates       []string             `json:"secret_candidates,omitempty"`
	SecretSources          map[string]string    `json:"secret_sources,omitempty"`
	Ports                  []PortEvidence       `json:"ports,omitempty"`
	HealthChecks           []Evidence           `json:"health_checks,omitempty"`
	Declared               []CapabilityIntent   `json:"declared_capabilities,omitempty"`
	Reconciliation         []ReconciliationItem `json:"reconciliation,omitempty"`
}

type Snapshot struct {
	Root  string
	Files map[string][]byte
}

type Detector interface {
	Name() string
	Detect(context.Context, Snapshot) ([]Finding, error)
}

type Engine struct {
	Detectors []Detector
}

type ComposeAnalysis struct {
	WorkloadServices       []string       `json:"workload_services,omitempty"`
	InfrastructureServices []string       `json:"infrastructure_services,omitempty"`
	AmbiguousServices      []string       `json:"ambiguous_services,omitempty"`
	SQLInstances           []string       `json:"sql_instances,omitempty"`
	CacheInstances         []string       `json:"cache_instances,omitempty"`
	ObjectStorageServices  []string       `json:"object_storage_services,omitempty"`
	Ports                  []PortEvidence `json:"ports,omitempty"`
	HealthChecks           []Evidence     `json:"health_checks,omitempty"`
}
