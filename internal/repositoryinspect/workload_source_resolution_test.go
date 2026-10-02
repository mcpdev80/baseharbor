package repositoryinspect

import "testing"

func TestWorkloadSourceResolutionNotDetected(t *testing.T) {
	got := ResolveWorkloadSource(nil, nil)
	if got.SchemaVersion != "baseharbor.workload-source-resolution/v1" ||
		got.State != WorkloadSourceResolutionNotDetected ||
		got.Reason != WorkloadSourceReasonNoSupportedSource ||
		got.CandidateCount != 0 ||
		got.Selected != nil {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionSingleCandidate(t *testing.T) {
	candidates := []WorkloadSourceCandidate{{Kind: WorkloadSourceCompose, Path: "compose.yaml"}}
	got := ResolveWorkloadSource(candidates, nil)
	if got.State != WorkloadSourceResolutionSelected ||
		got.Reason != WorkloadSourceReasonSingleCandidate ||
		got.Selected == nil ||
		got.Selected.Path != "compose.yaml" {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionProductionCandidateDominates(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "docker/docker-compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "e2e/docker-compose.yml"},
	}
	got := ResolveWorkloadSource(candidates, nil)
	if got.State != WorkloadSourceResolutionSelected ||
		got.Reason != WorkloadSourceReasonProductionCandidateDominates ||
		got.Selected == nil ||
		got.Selected.Path != "docker/docker-compose.yml" {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionCrossFamilyAmbiguous(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "compose.yaml"},
		{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"},
	}
	got := ResolveWorkloadSource(candidates, nil)
	if got.State != WorkloadSourceResolutionAmbiguous ||
		got.Reason != WorkloadSourceReasonCrossFamilyAmbiguity ||
		got.Selected != nil {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionLowConfidenceAmbiguous(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "scripts/compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "packages/client/compose.yml"},
	}
	got := ResolveWorkloadSource(candidates, nil)
	if got.State != WorkloadSourceResolutionAmbiguous ||
		got.Reason != WorkloadSourceReasonOnlyLowConfidenceCandidates ||
		got.Selected != nil {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionExplicitSelection(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "compose.yaml"},
		{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"},
	}
	explicit := &WorkloadSourceCandidate{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"}
	got := ResolveWorkloadSource(candidates, explicit)
	if got.State != WorkloadSourceResolutionSelected ||
		got.Reason != WorkloadSourceReasonExplicitRepositorySelection ||
		got.Selected == nil ||
		got.Selected.Kind != WorkloadSourceKubernetes {
		t.Fatalf("resolution = %#v", got)
	}
}

func TestWorkloadSourceResolutionInvalidExplicitSelection(t *testing.T) {
	candidates := []WorkloadSourceCandidate{{Kind: WorkloadSourceCompose, Path: "compose.yaml"}}
	explicit := &WorkloadSourceCandidate{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"}
	got := ResolveWorkloadSource(candidates, explicit)
	if got.State != WorkloadSourceResolutionInvalid ||
		got.Reason != WorkloadSourceReasonInvalidRepositoryMetadata ||
		got.Selected != nil {
		t.Fatalf("resolution = %#v", got)
	}
}
