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
)

type Evidence struct {
	Kind   EvidenceKind `json:"kind"`
	Path   string       `json:"path"`
	Detail string       `json:"detail"`
}

type Finding struct {
	Capability string     `json:"capability"`
	Name       string     `json:"name,omitempty"`
	Confidence Confidence `json:"confidence"`
	Evidence   []Evidence `json:"evidence"`
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
	Root              string            `json:"root"`
	Application       string            `json:"application"`
	ExistingManifest  string            `json:"existing_manifest,omitempty"`
	Artifacts         []Artifact        `json:"artifacts,omitempty"`
	ComposeCandidates []string          `json:"compose_candidates,omitempty"`
	SelectedCompose   string            `json:"selected_compose,omitempty"`
	WorkloadServices  []string          `json:"workload_services,omitempty"`
	Findings          []Finding         `json:"findings,omitempty"`
	SecretCandidates  []string          `json:"secret_candidates,omitempty"`
	SecretSources     map[string]string `json:"secret_sources,omitempty"`
	Ports             []PortEvidence    `json:"ports,omitempty"`
	HealthChecks      []Evidence        `json:"health_checks,omitempty"`
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
	WorkloadServices  []string       `json:"workload_services,omitempty"`
	PostgresInstances []string       `json:"postgres_instances,omitempty"`
	RedisInstances    []string       `json:"redis_instances,omitempty"`
	Ports             []PortEvidence `json:"ports,omitempty"`
	HealthChecks      []Evidence     `json:"health_checks,omitempty"`
}
