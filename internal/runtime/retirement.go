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
	}
	// An interrupted Down may have applied only part of the approved change.
	// Inspect actual disposition even on failure; initial inventory is not a
	// receipt proving preservation after a mutation.
	downErr := provider.DownProject(ctx, files.Project, files.Compose, files.Env)
	after, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		return result, errors.Join(downErr, fmt.Errorf("verify retired runtime; retain ownership state for retry: %w", err))
	}
	remaining := map[ProjectResource]bool{}
	var verificationErr error
	for _, resource := range after {
		if !seen[resource] {
			return result, errors.New("runtime retirement verification contains an unplanned resource")
		}
		remaining[resource] = true
		if resource.Kind == "container" {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("retired runtime container remains: %s", resource.Name))
		}
		if resource.Kind == "volume" {
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: "persistent data verified at its existing volume; deletion requires explicit consent and ownership proof"})
		}
		if resource.Kind == "network" {
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: "network remains at its existing owner; preserve connected consumers and retry state"})
		}
	}
	for _, resource := range before {
		if resource.Kind == "volume" && !remaining[resource] {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("runtime retirement lost persistent volume %s; retain ownership state for recovery", resource.Name))
		}
		if !remaining[resource] {
			result.Removed = append(result.Removed, resource)
		}
	}
	if downErr != nil {
		downErr = fmt.Errorf("retire owned runtime; retain ownership state for retry: %w", downErr)
	}
	return result, errors.Join(downErr, verificationErr)
}
