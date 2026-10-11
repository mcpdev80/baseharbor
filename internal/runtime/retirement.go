package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RuntimeRetirementProvider uses the existing runtime owner. The caller must
// prove from the authoritative consumer plan that this entire module is unused
// and supply its canonical resource inventory, never a discovered foreign file.
type RuntimeRetirementProvider interface {
	InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error)
	DownProject(context.Context, string, string, string) error
}

type RetainedRuntimeResource struct {
	Resource ProjectResource `json:"resource"`
	Reason   string          `json:"reason"`
}

// RuntimeRetirement is evidence, not another persisted ownership record. On
// failure callers keep their existing registry/deployment records for retry.
type RuntimeRetirement struct {
	Removed  []ProjectResource         `json:"removed"`
	Retained []RetainedRuntimeResource `json:"retained"`
}

// RetireOwnedRuntime removes an unused runtime module without deleting data.
// All container, network and volume ownership is checked before any mutation.
// Existing DownProject implementations preserve volumes on Docker and Quadlet.
func RetireOwnedRuntime(ctx context.Context, provider RuntimeRetirementProvider, files Files, expected []ProjectResource) (RuntimeRetirement, error) {
	result := RuntimeRetirement{Removed: []ProjectResource{}, Retained: []RetainedRuntimeResource{}}
	if provider == nil || strings.TrimSpace(files.Project) == "" || strings.TrimSpace(files.Compose) == "" || strings.TrimSpace(files.Env) == "" || len(expected) == 0 {
		return result, errors.New("runtime retirement requires provider, canonical files and resource inventory")
	}
	seen := map[ProjectResource]bool{}
	for _, resource := range expected {
		if strings.TrimSpace(resource.Name) == "" || (resource.Kind != "container" && resource.Kind != "network" && resource.Kind != "volume") || seen[resource] {
			return result, errors.New("runtime retirement inventory is invalid or duplicated")
		}
		seen[resource] = true
	}
	before, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		return result, fmt.Errorf("verify runtime retirement ownership: %w", err)
	}
	for _, resource := range before {
		if !seen[resource] {
			return result, errors.New("runtime retirement observation contains an unplanned resource")
		}
		if resource.Kind == "volume" {
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: "persistent data preserved; deletion requires explicit consent and ownership proof"})
		}
	}
	if err := provider.DownProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return result, fmt.Errorf("retire owned runtime; retain ownership state for retry: %w", err)
	}
	after, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		return result, fmt.Errorf("verify retired runtime; retain ownership state for retry: %w", err)
	}
	remaining := map[ProjectResource]bool{}
	for _, resource := range after {
		if !seen[resource] {
			return result, errors.New("runtime retirement verification contains an unplanned resource")
		}
		remaining[resource] = true
		if resource.Kind == "container" {
			return result, fmt.Errorf("retired runtime container remains: %s", resource.Name)
		}
		if resource.Kind == "network" {
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: "network remains in use; runtime owner preserved connected consumers"})
		}
	}
	for _, resource := range before {
		if resource.Kind == "volume" && !remaining[resource] {
			return result, fmt.Errorf("runtime retirement lost persistent volume %s; retain ownership state for recovery", resource.Name)
		}
		if !remaining[resource] {
			result.Removed = append(result.Removed, resource)
		}
	}
	return result, nil
}
