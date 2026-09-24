package artifact

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/workload"
)

// ResolveWorkload classifies normalized workload services into existing OCI
// artifacts or source build requests. Runtime providers consume only the
// resulting artifacts; they do not own repository build semantics.
func ResolveWorkload(repositoryRoot string, model workload.Model) (Resolution, error) {
	resolution := Resolution{}
	for _, service := range model.Services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			return Resolution{}, fmt.Errorf("workload service name is required for artifact resolution")
		}

		if service.Build != nil {
			contextPath, err := resolveSourcePath(repositoryRoot, service.Build.Context)
			if err != nil {
				return Resolution{}, fmt.Errorf("service %s build context: %w", name, err)
			}
			dockerfile := strings.TrimSpace(service.Build.Dockerfile)
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}
			resolution.Builds = append(resolution.Builds, BuildRequest{
				Service:       name,
				Context:       contextPath,
				Dockerfile:    filepath.ToSlash(dockerfile),
				ReferenceHint: strings.TrimSpace(service.Image),
			})
			continue
		}

		reference := strings.TrimSpace(service.Image)
		if reference == "" {
			return Resolution{}, fmt.Errorf(
				"workload service %q has neither an OCI image reference nor source build input",
				name,
			)
		}
		resolution.Artifacts = append(resolution.Artifacts, Artifact{
			Service:   name,
			Reference: reference,
		})
	}
	return resolution, nil
}

func resolveSourcePath(repositoryRoot, context string) (string, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(context)
	if value == "" {
		value = "."
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("absolute source paths are not portable: %s", value)
	}
	target, err := filepath.Abs(filepath.Join(root, value))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path escapes repository root: %s", value)
	}
	return target, nil
}
