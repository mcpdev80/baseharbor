package repositoryinspect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const RepositoryMetadataName = "baseharbor.repository.yaml"

type RepositoryMetadata struct {
	Version        int                              `yaml:"version" json:"version"`
	WorkloadSource RepositoryWorkloadSourceMetadata `yaml:"workload-source" json:"workload_source"`
}

type RepositoryWorkloadSourceMetadata struct {
	Kind WorkloadSourceKind `yaml:"kind" json:"kind"`
	Path string             `yaml:"path" json:"path"`
}

func ParseRepositoryMetadata(data []byte) (RepositoryMetadata, error) {
	var metadata RepositoryMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return RepositoryMetadata{}, fmt.Errorf("decode %s: %w", RepositoryMetadataName, err)
	}
	if metadata.Version != 1 {
		return RepositoryMetadata{}, fmt.Errorf("%s version %d is unsupported; expected 1", RepositoryMetadataName, metadata.Version)
	}
	switch metadata.WorkloadSource.Kind {
	case WorkloadSourceCompose, WorkloadSourceQuadlet, WorkloadSourceKubernetes:
	default:
		return RepositoryMetadata{}, fmt.Errorf("%s workload-source kind %q is unsupported", RepositoryMetadataName, metadata.WorkloadSource.Kind)
	}
	path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(metadata.WorkloadSource.Path)))
	if path == "" || path == ".." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return RepositoryMetadata{}, fmt.Errorf("%s workload-source path must stay inside the repository", RepositoryMetadataName)
	}
	metadata.WorkloadSource.Path = path
	return metadata, nil
}

func repositoryMetadataFromSnapshot(snapshot Snapshot) (*RepositoryMetadata, error) {
	data, ok := snapshot.Files[RepositoryMetadataName]
	if !ok {
		return nil, nil
	}
	metadata, err := ParseRepositoryMetadata(data)
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

func selectWorkloadSourceWithMetadata(candidates []WorkloadSourceCandidate, metadata *RepositoryMetadata) (*WorkloadSourceCandidate, error) {
	if metadata == nil {
		return selectWorkloadSource(candidates), nil
	}
	for i := range candidates {
		candidate := candidates[i]
		if candidate.Kind == metadata.WorkloadSource.Kind && filepath.ToSlash(candidate.Path) == metadata.WorkloadSource.Path {
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("%s selects %s %s, but that source was not detected", RepositoryMetadataName, metadata.WorkloadSource.Kind, metadata.WorkloadSource.Path)
}

func WriteRepositoryMetadata(root string, candidate WorkloadSourceCandidate) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	metadata := RepositoryMetadata{
		Version: 1,
		WorkloadSource: RepositoryWorkloadSourceMetadata{
			Kind: candidate.Kind,
			Path: filepath.ToSlash(candidate.Path),
		},
	}
	if _, err := selectWorkloadSourceWithMetadata([]WorkloadSourceCandidate{candidate}, &metadata); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, RepositoryMetadataName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}
