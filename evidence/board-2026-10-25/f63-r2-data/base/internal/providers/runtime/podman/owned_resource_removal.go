package podman

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (p PodmanProvider) DestroyOwnedProjectResources(ctx context.Context, project string, resources []ProjectResource) error {
	return p.removeOwnedProjectResources(ctx, project, resources, true)
}

// RetireOwnedProjectContainers stops owned Quadlet units and preserves all
// anonymous and named data volumes. It never passes a volume removal request.
func (p PodmanProvider) RetireOwnedProjectContainers(ctx context.Context, project string, resources []ProjectResource) error {
	for _, resource := range resources {
		if resource.Kind != "container" {
			return errors.New("runtime retirement accepts only containers")
		}
	}
	dir, err := quadletUserUnitDir()
	if err != nil {
		return err
	}
	definitions, err := plannedQuadletRetirement(dir, project, resources)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if err := verifyRetiredQuadletDefinition(definition); err != nil {
			return err
		}
	}
	if err := p.removeOwnedProjectResources(ctx, project, resources, false); err != nil {
		return err
	}
	if err := removeRetiredQuadletDefinitions(ctx, definitions); err != nil {
		return err
	}
	// Retry must clear an earlier generator cache even if the container and
	// its definition were already removed before daemon-reload failed.
	if len(resources) > 0 && len(definitions) == 0 {
		_, err = quadletSystemctlBlocking(ctx, nil, "daemon-reload")
		return err
	}
	return nil
}

func (p PodmanProvider) removeOwnedProjectResources(ctx context.Context, project string, resources []ProjectResource, deleteAnonymousVolumes bool) error {
	existing, err := p.InspectProjectResources(ctx, project, resources)
	if err != nil {
		return err
	}

	var containerUnits []string
	seenUnits := map[string]struct{}{}
	for _, resource := range existing {
		if resource.Kind != "container" {
			continue
		}
		actualName, exists, resolveErr := p.resolveOwnedContainerResourceName(ctx, project, resource.Name)
		if resolveErr != nil {
			return resolveErr
		}
		if !exists {
			continue
		}
		unitOut, inspectErr := p.DirectOutput(ctx,
			"container", "inspect", "--format",
			`{{ index .Config.Labels "PODMAN_SYSTEMD_UNIT" }}`,
			actualName,
		)
		if inspectErr != nil {
			return fmt.Errorf("inspect Quadlet unit for owned container %s: %w", resource.Name, inspectErr)
		}
		unit := podmanSystemdUnitLabel(unitOut)
		if unit == "" {
			continue
		}
		if !deleteAnonymousVolumes {
			expected := strings.TrimSuffix(resource.Name, "-1") + ".service"
			if unit != expected {
				return errors.New("selected container claims a foreign Quadlet service unit")
			}
		}
		if _, seen := seenUnits[unit]; seen {
			continue
		}
		seenUnits[unit] = struct{}{}
		containerUnits = append(containerUnits, unit)
	}
	sort.Strings(containerUnits)
	if len(containerUnits) > 0 {
		if _, stopErr := quadletSystemctlCombined(ctx, append([]string{"stop"}, containerUnits...)...); stopErr != nil {
			return fmt.Errorf("stop owned Quadlet services before resource removal for project %s: %w", project, stopErr)
		}
	}

	for _, kind := range []string{"container", "network", "volume"} {
		for _, resource := range existing {
			if resource.Kind != kind {
				continue
			}
			actualName := resource.Name
			if kind == "container" {
				resolvedName, exists, resolveErr := p.resolveOwnedContainerResourceName(ctx, project, resource.Name)
				if resolveErr != nil {
					return resolveErr
				}
				if !exists {
					continue
				}
				actualName = resolvedName
			}
			args := []string{kind, "rm", actualName}
			if kind == "container" {
				args = []string{"container", "rm", "-f"}
				if deleteAnonymousVolumes {
					args = append(args, "-v")
				}
				args = append(args, actualName)
			}
			if _, err := p.DirectOutput(ctx, args...); err != nil {
				if kind == "network" && podmanNetworkHasActiveConsumers(err) {
					continue
				}
				remaining, inspectErr := p.InspectProjectResources(ctx, project, []ProjectResource{resource})
				if inspectErr == nil && len(remaining) == 0 {
					continue
				}
				if inspectErr != nil {
					return fmt.Errorf("remove owned %s %s: %w (post-remove verification failed: %v)", kind, resource.Name, err, inspectErr)
				}
				return fmt.Errorf("remove owned %s %s: %w", kind, resource.Name, err)
			}
		}
	}
	return nil
}
